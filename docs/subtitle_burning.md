# Subtitle Burning

ViralClip AI can burn (hard-code) captions directly into clip video files so
that subtitles are visible on every platform without requiring a separate `.srt`
upload.

---

## How It Works

```
┌─────────┐          ┌───────────┐         ┌────────────────────┐
│  Web UI  │ ──POST──▶│  Go API   │──POST──▶│  Python AI Service │
│          │          │ /subtitles│         │  /process/subtitles│
│          │          │  /burn    │         │                    │
│ CC badge │◀─JSON────│           │◀─JSON───│  FFmpeg burns each │
│ updates  │          │  updates  │         │  clip_XXX.mp4      │
└─────────┘          │  DB clips │         └────────────────────┘
                      └───────────┘
```

1. User clicks **Burn Subtitles** on a completed video in the web dashboard.
2. The Go API verifies ownership and that the video status is `completed`.
3. The Go API calls `POST /process/subtitles` on the Python AI service,
   forwarding optional style overrides.
4. The AI service reads the clip manifest and cached transcript from disk,
   filters transcript segments to each clip's time window, and calls
   `ffmpeg -vf subtitles=...` to burn captions into each extracted clip.
5. The Go API marks all clips for the video with `subtitle_path = "subtitled"`.
6. The web frontend invalidates the clips query cache so the **CC** badge
   appears on each clip immediately.

---

## API

### Burn subtitles into all clips of a video

```
POST /api/v1/videos/:videoId/subtitles/burn
Authorization: Bearer <access_token>
Content-Type: application/json
```

**Request body** (all fields optional):

| Field           | Type    | Default       | Description                                         |
|-----------------|---------|---------------|-----------------------------------------------------|
| `style`         | string  | `"default"`   | Visual style: `default`, `bold`, `outline`, `shadow`|
| `font_size`     | integer | `24`          | Font size in points (12–72)                         |
| `primary_color` | string  | `"&H00FFFFFF"`| Text colour in ASS/SSA `&HBBGGRR` format            |
| `outline_color` | string  | `"&H00000000"`| Outline colour in ASS/SSA `&HBBGGRR` format         |

**Successful response `200`:**

```json
{
  "success": true,
  "data": {
    "video_id": "550e8400-e29b-41d4-a716-446655440000",
    "clips_processed": 4
  }
}
```

**Error responses:**

| Status | Reason                                             |
|--------|----------------------------------------------------|
| `400`  | Invalid video ID or video not yet `completed`      |
| `401`  | Not authenticated                                  |
| `404`  | Video not found or not owned by the user           |
| `500`  | AI service unavailable or subtitle burning failed  |

**curl example:**

```bash
curl -X POST http://localhost:8080/api/v1/videos/<VIDEO_ID>/subtitles/burn \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"style":"bold","font_size":28}'
```

---

## Subtitle Styles

| Style     | Description                                               |
|-----------|-----------------------------------------------------------|
| `default` | White text, semi-transparent black box, slight outline    |
| `bold`    | Bold white text, no background box                        |
| `outline` | White text with a thick border, no shadow                 |
| `shadow`  | White text with a drop shadow, no background box          |

---

## Colour Format

Colours use FFmpeg's ASS/SSA notation: `&HBBGGRR` (hexadecimal, reversed
channel order compared to CSS `#RRGGBB`).

Examples:

| Colour        | ASS/SSA value   |
|---------------|-----------------|
| White         | `&H00FFFFFF`    |
| Black         | `&H00000000`    |
| Yellow        | `&H0000FFFF`    |
| Semi-transparent black overlay | `&H80000000` |

---

## Prerequisites

- The source video must be in `completed` status (transcription and clip
  extraction must have finished).
- The AI service must have FFmpeg installed with libass subtitle support.
- The AI service filesystem must still contain the extracted clip files and
  the cached transcript JSON produced during video processing.

---

## Frontend Integration

The web dashboard exposes subtitle burning on the **Video Detail** page
(`/videos/:id`).  The section is visible only when:

- `video.status === "completed"`
- At least one clip exists for the video

After burning, each clip shows a cyan **CC** badge in the clips list.

The `useBurnSubtitles(videoId)` React hook (in `apps/web/src/hooks/useSubtitles.ts`)
can be used to trigger burning from any component:

```ts
import { useBurnSubtitles } from "@/hooks/useSubtitles";

const burnSubtitles = useBurnSubtitles(videoId);

burnSubtitles.mutate({ style: "bold", font_size: 28 });
```
