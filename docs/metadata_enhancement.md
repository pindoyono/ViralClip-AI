# AI-Powered Clip Metadata Enhancement

## Overview

Task 5 adds an **AI-powered metadata enhancement** workflow that lets users
generate platform-optimised titles, descriptions, and hashtags for any ready
clip with a single API call.

The feature chains the existing **AI metadata service** (running in the Python
FastAPI process) with a new Go API endpoint, and surfaces a "✨ Enhance" button
per clip on the video detail page in the web frontend.

---

## Architecture

```
Web Frontend
  │  POST /api/v1/clips/:id/metadata/enhance
  ▼
Go Fiber API (MetadataHandler)
  │  POST <ai-service-url>/api/v1/metadata
  ▼
Python FastAPI AI Service (metadata router → metadata_service.py)
  │  GPT-4 chat completion
  ▼
OpenAI API
```

The AI service already existed (`apps/ai-service/app/routers/metadata.py`);
this task wires it up from the Go API and the web frontend.

---

## API Endpoint

### Enhance Clip Metadata

```
POST /api/v1/clips/:id/metadata/enhance
Authorization: Bearer <access_token>
Content-Type: application/json
```

**Request body (optional):**

| Field      | Type   | Required | Description |
|------------|--------|----------|-------------|
| `platform` | string | No       | Target platform: `tiktok` \| `instagram` \| `youtube` \| `twitter`. Defaults to `"tiktok"`. |
| `niche`    | string | No       | Content niche (e.g. `"tech"`, `"fitness"`). |
| `tone`     | string | No       | Tone descriptor (e.g. `"educational"`, `"humorous"`). |

**Response `200`:**

```json
{
  "success": true,
  "data": {
    "clip": {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "title": "5 React Hooks You Didn't Know Existed",
      "description": "Unlock the full power of React with these lesser-known hooks...",
      "hashtags": ["react", "webdev", "javascript", "coding", "fyp"],
      ...
    },
    "keywords": ["react", "hooks", "javascript", "frontend", "programming"],
    "category": "Education",
    "optimal_post_times": [
      "7:00 PM EST on Weekdays",
      "12:00 PM EST on Weekends"
    ]
  }
}
```

**curl example:**

```bash
curl -X POST http://localhost:8080/api/v1/clips/<clip-id>/metadata/enhance \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"platform":"tiktok","niche":"tech","tone":"educational"}'
```

**Errors:**

| Status | Code | Reason |
|--------|------|--------|
| `401` | `unauthorized` | Missing or invalid token |
| `404` | `not_found` | Clip not found or not owned by user |
| `400` | `bad_request` | Malformed request body |
| `500` | `internal_error` | AI service unavailable or returned an error |

---

## How It Works

1. The Go handler loads the clip from the database and verifies ownership.
2. It assembles a **pseudo-transcript** from the clip's existing metadata
   fields (`title`, `hook_text`, `description`, `ai_rationale`, `hashtags`)
   so the AI has rich context without needing the full video transcript file.
3. The handler calls `POST <ai-service>/api/v1/metadata` with the assembled
   text, target platform, and optional niche/tone overrides.
4. The AI service (GPT-4) returns an optimised `title`, `description`,
   `hashtags`, `keywords`, `category`, and `optimal_post_times`.
5. The handler persists the updated `title`, `description`, and `hashtags`
   to the clip record in PostgreSQL.
6. The full response (including non-persisted `keywords`, `category`, and
   `optimal_post_times`) is returned to the caller.

---

## Hashtag Limits by Platform

The AI service enforces platform-specific hashtag caps:

| Platform   | Max hashtags |
|------------|-------------|
| TikTok     | 10          |
| Instagram  | 30          |
| YouTube    | 15          |
| Twitter    | 3           |

---

## Frontend Integration

The "✨ Enhance" button appears on every `ready` clip card in the video detail
page (`/videos/:id`).

1. A **platform selector** above the clip list lets the user choose the
   target platform for all Enhance calls on that page.
2. Clicking **✨ Enhance** on a clip sends the API request with the selected
   platform.
3. On success, an inline result panel shows the new title, category, keywords,
   and optimal posting time.
4. React Query cache keys for the clip and its parent video's clip list are
   invalidated so the updated title and hashtags are reflected immediately.

---

## Tests

| Layer | Location | Tests |
|-------|----------|-------|
| Go API handler | `apps/api/internal/handlers/metadata_test.go` | 8 tests |
| Web hook | `apps/web/src/__tests__/useMetadata.test.ts` | 5 tests |

---

## AI Service (unchanged)

The existing AI service endpoint at `POST /api/v1/metadata` accepts:

```json
{
  "video_id": "<clip-id>",
  "transcript": "<text context>",
  "platform": "tiktok",
  "niche": "tech",
  "tone": "educational"
}
```

No changes to the Python AI service were required.
