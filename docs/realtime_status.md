# Real-Time Video Processing Status

> Task 4 — WebSocket + HTTP polling pipeline status updates

ViralClip AI exposes two complementary mechanisms to let clients track the
video processing pipeline in real time:

| Mechanism | Endpoint | Use case |
|-----------|----------|----------|
| **HTTP polling** | `GET /api/v1/videos/:id/job-status` | Fallback for environments where WebSocket is unavailable |
| **WebSocket push** | `GET /ws?token=<jwt>` | Live updates with sub-second latency |

---

## Architecture

```
Worker pipeline                  Redis              API (Go Fiber)          Browser
─────────────────────────────────────────────────────────────────────────────────────

TranscriptWorker ──PUBLISH──► video:status:{id} ──SUBSCRIBE──► StatusBroadcaster
ClipWorker       ──PUBLISH──►                                           │
SubtitleWorker   ──PUBLISH──►                                           │
UploadWorker     ──PUBLISH──►                                    WebSocket Hub
                                                                        │
                                                                 SendToUser(userID)
                                                                        │
Worker also sets ──────────────► job:{id} = "stage:status" ─── GET /job-status ─► Browser
```

### Pipeline Stages

| Stage | Label | Redis key value |
|-------|-------|----------------|
| `transcript` | Transcription | `transcript:processing` → `transcript:done` |
| `clip` | Clip Generation | `clip:processing` → `clip:done` |
| `subtitle` | Subtitle Burning | `subtitle:processing` → `subtitle:done` |
| `upload` | Finalising | `upload:processing` → `upload:done` |
| `completed` | *(terminal)* | `upload:done` + `video.status=completed` |

---

## HTTP API

### `GET /api/v1/videos/:id/job-status`

**Authentication:** JWT bearer token required.

**Response:**

```json
{
  "success": true,
  "data": {
    "video_id": "uuid",
    "video_status": "processing",
    "job_status": "clip:processing",
    "current_stage": "clip",
    "stages": [
      { "stage": "transcript", "status": "done",       "label": "Transcription"  },
      { "stage": "clip",       "status": "processing", "label": "Clip Generation" },
      { "stage": "subtitle",   "status": "pending",    "label": "Subtitle Burning" },
      { "stage": "upload",     "status": "pending",    "label": "Finalising"      }
    ]
  }
}
```

**Stage status values:** `pending` | `processing` | `done` | `failed` | `skipped`

**Video status values:** `pending` | `processing` | `completed` | `failed`

---

## WebSocket API

### `GET /ws?token=<jwt>`

Upgrade a plain HTTP request to a WebSocket connection.

**Authentication:** pass the JWT access token as the `token` query parameter.
Standard `Authorization` headers are not forwarded during the WebSocket
handshake by browsers.

**Server → Client messages**

```jsonc
{
  "type": "status_update",
  "video_id": "uuid",
  "payload": {
    // Same shape as JobStatusResponse above
  }
}
```

The server only pushes messages for videos whose processing events are
received via Redis Pub/Sub. Clients are responsible for subscribing to the
stages they care about by filtering on `video_id`.

---

## Frontend Integration

### Polling hook (`useJobStatus`)

```ts
import { useJobStatus } from "@/hooks/useJobStatus";

// Polls every 3 s while the video is processing; stops automatically on
// terminal states (completed / failed).
const { data: jobStatus } = useJobStatus(videoId);
```

### WebSocket hook (`useVideoProcessingWS`)

```ts
import { useVideoProcessingWS } from "@/hooks/useJobStatus";

const { connected } = useVideoProcessingWS(videoId, {
  onUpdate: (payload) => {
    // payload is JobStatusResponse
    console.log(payload.current_stage, payload.video_status);
  },
});
```

The hook reads the JWT from `localStorage.access_token` (set by the auth
store on login) and constructs `ws://API_HOST/ws?token=<jwt>`.

---

## Worker Integration

Each pipeline worker (`TranscriptWorker`, `ClipWorker`, `SubtitleWorker`,
`UploadWorker`) now holds a `StatusPublisher` that publishes two events per
stage:

1. **Start**: `stage:processing` + `video_status:processing`
2. **End**: `stage:done` / `stage:failed` + correct `video_status`

Workers are initialized with a no-op publisher by default. Pass a real
`StatusPublisher` via `WithStatusPublisher(pub)`:

```go
pub := workers.NewStatusPublisher(rdb)
transcriptWorker := workers.NewTranscriptWorker(db, qCli, aiURL, maxRetries).
    WithStatusPublisher(pub)
```

This is wired automatically in `apps/worker/main.go`.

---

## Testing

### Go API (`apps/api`)

```bash
cd apps/api
go test ./internal/handlers/... -run "TestGetJobStatus|TestBuildJobStatus|TestWSUpgrade"
```

### Go Worker (`apps/worker`)

All existing `queue_workers` tests continue to pass. The `StatusPublisher`
is a no-op in tests (nil Redis client).

### Web (`apps/web`)

```bash
cd apps/web
npx jest --testPathPattern="useJobStatus"
```

---

## Operational Notes

- The Redis job status key `job:{videoID}` has a 24-hour TTL. After expiry
  the HTTP endpoint returns an empty `job_status` string and derives the stage
  from `video.status` only.
- The WebSocket hub is a single in-process map. In a multi-instance
  deployment, ensure sticky sessions (Nginx `ip_hash`) or replace the hub
  with a Redis-backed fan-out.
- Clients that cannot use WebSocket (server-side rendering, some proxies) can
  rely entirely on the HTTP polling endpoint.
