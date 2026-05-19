# Auto Publishing Engine

This document describes the complete Auto Publishing Engine for ViralClip AI — the subsystem responsible for scheduling, uploading, and tracking clip publications to YouTube Shorts, Instagram Reels, and TikTok.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Database Schema](#database-schema)
3. [Worker Components](#worker-components)
   - [SchedulerWorker](#schedulerworker)
   - [PublishingWorker](#publishingworker)
   - [TokenRefreshService](#tokenrefreshservice)
4. [Platform Uploaders](#platform-uploaders)
5. [Upload Progress Tracking](#upload-progress-tracking)
6. [API Endpoints](#api-endpoints)
7. [OAuth Token Lifecycle](#oauth-token-lifecycle)
8. [Retry & Failure Handling](#retry--failure-handling)
9. [Configuration](#configuration)
10. [Running Migrations](#running-migrations)
11. [Development Notes](#development-notes)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Worker Process                           │
│                                                                 │
│  ┌──────────────────┐  every 30 s   ┌─────────────────────┐    │
│  │  SchedulerWorker │──────────────▶│   scheduled_posts   │    │
│  │                  │  status →     │   status: scheduled │    │
│  │ monitors due     │  "publishing" │   → publishing      │    │
│  │ scheduled posts  │               └─────────────────────┘    │
│  └──────────────────┘                         │                 │
│                                               │ every 60 s     │
│  ┌──────────────────┐                         ▼                 │
│  │  PublishingWorker│◀──────────── picks "publishing" posts     │
│  │                  │                                           │
│  │ 1. load clip     │──▶ PlatformUploader ──▶ YouTube / IG /    │
│  │ 2. refresh token │       (per platform)     TikTok API       │
│  │ 3. upload clip   │                                           │
│  │ 4. update status │──▶ status → "published"                   │
│  │ 5. write logs    │──▶ publishing_logs                        │
│  └──────────────────┘                                           │
│                                                                 │
│  ┌──────────────────┐  every 15 min                             │
│  │TokenRefreshSvc   │──▶ proactive token refresh                │
│  │                  │──▶ social_accounts.access_token updated   │
│  │ refresh expiring │──▶ failures → Redis pub/sub               │
│  │ OAuth tokens     │    channel: token:refresh:failures        │
│  └──────────────────┘                                           │
└─────────────────────────────────────────────────────────────────┘
```

---

## Database Schema

### `social_accounts`

| Column                  | Type         | Description                                      |
|-------------------------|--------------|--------------------------------------------------|
| `id`                    | UUID         | Primary key                                      |
| `user_id`               | UUID         | Foreign key → `users.id`                         |
| `platform`              | VARCHAR(50)  | `tiktok` \| `instagram` \| `youtube`             |
| `platform_user_id`      | VARCHAR(255) | Platform-assigned user/channel ID                |
| `username`              | VARCHAR(255) | Display username                                 |
| `access_token`          | TEXT         | OAuth 2.0 access token (encrypted at rest)       |
| `refresh_token`         | TEXT         | OAuth 2.0 refresh token (encrypted at rest)      |
| `expires_at`            | TIMESTAMPTZ  | Access token expiry time                         |
| `is_active`             | BOOLEAN      | Whether the connection is active                 |
| `token_refresh_attempts`| INTEGER      | Consecutive refresh failures (reset on success)  |

### `scheduled_posts`

| Column              | Type         | Description                                                  |
|---------------------|--------------|--------------------------------------------------------------|
| `id`                | UUID         | Primary key                                                  |
| `clip_id`           | UUID         | Foreign key → `clips.id`                                     |
| `social_account_id` | UUID         | Foreign key → `social_accounts.id`                           |
| `user_id`           | UUID         | Foreign key → `users.id`                                     |
| `platform`          | VARCHAR(50)  | Target platform                                              |
| `caption`           | TEXT         | Post caption                                                 |
| `hashtags`          | TEXT         | Hashtags string                                              |
| `scheduled_at`      | TIMESTAMPTZ  | When the post was originally scheduled                       |
| `publish_at`        | TIMESTAMPTZ  | Target publication time (may be bumped on retry)             |
| `published_at`      | TIMESTAMPTZ  | Actual publication time                                      |
| `status`            | VARCHAR(50)  | `scheduled` \| `publishing` \| `published` \| `failed` \| `cancelled` |
| `platform_post_id`  | VARCHAR(255) | Platform-assigned post/video ID                              |
| `platform_post_url` | TEXT         | Public URL of the published post                             |
| `upload_progress`   | INTEGER      | Upload progress 0–100 (live, also tracked in Redis)          |
| `retry_count`       | INTEGER      | Number of publish attempts                                   |
| `error_message`     | TEXT         | Last error message                                           |

### `publishing_logs`

| Column     | Type        | Description                              |
|------------|-------------|------------------------------------------|
| `id`       | UUID        | Primary key                              |
| `post_id`  | UUID        | Foreign key → `scheduled_posts.id`       |
| `status`   | VARCHAR(50) | Status at the time of the log entry      |
| `message`  | TEXT        | Human-readable description of the event  |
| `created_at` | TIMESTAMPTZ | Timestamp of the log entry             |

---

## Worker Components

### SchedulerWorker

**File:** `apps/worker/internal/workers/workers.go`  
**Run interval:** every 30 seconds

The `SchedulerWorker` is a lightweight monitor that transitions _due_ posts from `scheduled` → `publishing`, signalling to the `PublishingWorker` that they are ready to upload.

```
EnqueueDuePosts(ctx):
  SELECT * FROM scheduled_posts
  WHERE status IN ('scheduled', 'pending')
    AND COALESCE(publish_at, scheduled_at) <= NOW()
  LIMIT 50

  For each post:
    UPDATE scheduled_posts SET status = 'publishing' WHERE id = post.id
```

### PublishingWorker

**File:** `apps/worker/internal/workers/workers.go`  
**Run interval:** every 60 seconds

The `PublishingWorker` picks up posts in `publishing` state and drives them through the full upload pipeline:

1. Load the scheduled post.
2. Load the associated `SocialAccount`; ensure it is active.
3. Write a `publishing_logs` entry: "publishing started".
4. Ensure the access token is valid via `ensureValidAccessToken` (inline refresh if expired).
5. Load the `Clip` record to obtain the storage path / URL.
6. Delegate upload to the correct `PlatformUploader` for the platform.
7. Update `scheduled_posts`: `status = published`, `platform_post_id`, `platform_post_url`, `upload_progress = 100`.
8. Write a `publishing_logs` entry: "post published successfully".

On any error, `failPostWithRetry` is called:
- If `retry_count < maxRetries (3)`: status → `scheduled`, `publish_at` bumped by `2 * retry_count` minutes.
- If `retry_count >= maxRetries`: status → `failed` (permanent).

### TokenRefreshService

**File:** `apps/worker/internal/workers/token_refresh.go`  
**Run interval:** every 15 minutes (also runs once immediately on worker startup)

The `TokenRefreshService` proactively refreshes OAuth tokens that are within 15 minutes of expiry, ensuring the `PublishingWorker` always finds a valid access token.

```
RefreshExpiringTokens(ctx):
  SELECT * FROM social_accounts
  WHERE is_active = TRUE
    AND refresh_token != ''
    AND (expires_at IS NULL OR expires_at < NOW() + 15min)
  LIMIT 100

  For each account:
    newToken, newExpiry = callPlatformRefresh(account)
    if success:
      UPDATE social_accounts SET access_token = newToken, expires_at = newExpiry,
                                 token_refresh_attempts = 0
    if failure:
      INCREMENT token_refresh_attempts
      PUBLISH failure event to Redis: token:refresh:failures
```

On refresh failure the service publishes a JSON event to the Redis Pub/Sub channel `token:refresh:failures`:

```json
{
  "account_id": "uuid",
  "platform": "tiktok",
  "reason": "HTTP 401 from TikTok token endpoint",
  "ts": "2026-05-19T08:00:00Z"
}
```

---

## Platform Uploaders

**File:** `apps/worker/internal/workers/platform_uploader.go`

The `PlatformUploader` interface decouples the `PublishingWorker` from platform-specific upload logic:

```go
type PlatformUploader interface {
    Upload(ctx, post, account, clip) (platformPostID, platformPostURL string, err error)
}
```

Three implementations are provided as production-ready stubs:

| Uploader | Platform | Production API |
|---|---|---|
| `YouTubeShortUploader` | YouTube Shorts | YouTube Data API v3 — resumable upload |
| `InstagramReelUploader` | Instagram Reels | Instagram Graph API — media container upload |
| `TikTokUploader` | TikTok | TikTok Direct Post API — chunked video upload |

Each uploader:
1. Sets upload progress to 0 in Redis (`upload:progress:{postID}`).
2. Performs the upload (chunked, with progress updates at intermediate milestones).
3. Sets upload progress to 100 in Redis on completion.

**TODO for production:** Replace the stub implementations with real HTTP calls to each platform's API. The integration points are documented with `// TODO` comments in each struct.

---

## Upload Progress Tracking

Upload progress is tracked in two places:

1. **Redis** (real-time): key `upload:progress:{postID}` → integer 0–100, TTL 1 hour.  
   Clients can read this key for live progress without polling the DB.

2. **Database** (durable): `scheduled_posts.upload_progress` is set to 100 when the upload completes.

Helper functions:

```go
// Worker (write)
setUploadProgress(ctx, rdb, postID, pct int)

// API / polling client (read)
GetUploadProgress(ctx, rdb, postID string) int  // returns -1 if not found
```

---

## API Endpoints

All endpoints are under `/api/v1` and require JWT authentication.

### `POST /api/v1/social/connect` — Connect a social account

Request:
```json
{
  "platform": "tiktok",
  "username": "myhandle",
  "display_name": "My Handle",
  "access_token": "oauth_access_token",
  "refresh_token": "oauth_refresh_token",
  "expires_at": "2026-06-01T00:00:00Z",
  "followers_count": 50000
}
```

Response `201 Created`:
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "platform": "tiktok",
    "username": "myhandle",
    "is_active": true,
    ...
  }
}
```

### `POST /api/v1/social/disconnect` — Disconnect a social account

Request:
```json
{ "account_id": "uuid" }
```

Response `200 OK`:
```json
{ "success": true, "message": "Account disconnected successfully" }
```

### `POST /api/v1/schedule` — Schedule a clip for publishing

Request:
```json
{
  "clip_id": "uuid",
  "social_account_id": "uuid",
  "scheduled_at": "2026-06-01T10:00:00Z",
  "publish_at": "2026-06-01T10:00:00Z",
  "caption": "Check this out 🔥",
  "hashtags": "#viral #shorts"
}
```

Response `201 Created`:
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "status": "scheduled",
    "platform": "youtube",
    "publish_at": "2026-06-01T10:00:00Z",
    ...
  }
}
```

### `POST /api/v1/publish` — Publish immediately

Identical body to `/schedule` but `publish_at` is set to now, causing the `SchedulerWorker` to pick it up on its next tick (within 30 seconds).

### `GET /api/v1/publish/status?post_id={uuid}` — Get publish status

Response `200 OK`:
```json
{
  "success": true,
  "data": {
    "post": {
      "id": "uuid",
      "status": "publishing",
      "upload_progress": 66,
      "platform_post_url": null,
      ...
    },
    "logs": [
      { "id": "uuid", "status": "publishing", "message": "publishing started", "created_at": "..." }
    ]
  }
}
```

---

## OAuth Token Lifecycle

```
User connects account (POST /social/connect)
  └─▶ access_token + refresh_token stored in social_accounts

TokenRefreshService (every 15 min)
  └─▶ finds tokens expiring within 15 min
  └─▶ calls platform OAuth refresh endpoint
  └─▶ updates access_token + expires_at in DB
  └─▶ on failure: increments token_refresh_attempts, publishes to Redis

PublishingWorker (inline safety net)
  └─▶ ensureValidAccessToken: if token is expired → delegate to TokenRefreshService.RefreshAccountToken
  └─▶ TokenRefreshService updates social_accounts.access_token + expires_at
  └─▶ PublishingWorker reloads updated token and continues upload
  └─▶ on failure: failPostWithRetry
```

The `TokenRefreshService` is the primary refresh mechanism.  The inline refresh in `PublishingWorker.ensureValidAccessToken` is a safety net for cases where the service was unable to refresh in advance.

---

## Retry & Failure Handling

| Scenario | Action |
|---|---|
| Upload fails (transient) | `status → scheduled`, `publish_at` bumped by `2 × retry_count` minutes |
| Max retries (3) exceeded | `status → failed` (permanent), log written |
| Clip not found | Treated as permanent failure (after retries) |
| Account inactive / missing | Treated as transient failure (retried) |
| Token missing with no refresh_token | Treated as transient failure (retried) |
| Token expired, refresh succeeds | Continues upload normally |
| Token expired, refresh fails | Treated as transient failure (retried) |

---

## Configuration

All settings are read from environment variables (see `apps/worker/internal/config/config.go`):

| Variable | Default | Description |
|---|---|---|
| `WORKER_MAX_RETRIES` | `3` | Maximum upload attempts per post |
| `WORKER_CONCURRENCY` | `4` | Number of queue consumer goroutines |

Worker loop intervals are hard-coded in `apps/worker/main.go`:

| Loop | Interval | Description |
|---|---|---|
| SchedulerWorker | 30 s | Enqueues due posts |
| PublishingWorker | 60 s | Processes posts in `publishing` state |
| TokenRefreshService | 15 min | Proactive token refresh |

---

## Running Migrations

Migrations are plain SQL files in `infrastructure/migrations/`. Run them in order:

```bash
psql "$DATABASE_URL" -f infrastructure/migrations/005_create_social_accounts.sql
psql "$DATABASE_URL" -f infrastructure/migrations/006_create_scheduled_posts.sql
psql "$DATABASE_URL" -f infrastructure/migrations/010_auto_publishing_engine.sql
psql "$DATABASE_URL" -f infrastructure/migrations/012_publishing_engine_progress.sql
```

Migration `012` adds:
- `scheduled_posts.upload_progress` INTEGER DEFAULT 0 (0–100)
- `social_accounts.token_refresh_attempts` INTEGER DEFAULT 0
- Index on `social_accounts.expires_at` for the `TokenRefreshService` query

---

## Development Notes

### Implementing real platform uploads

Each `PlatformUploader` implementation contains `// TODO` comments that mark exactly where to add the real HTTP client code. The steps are:

**YouTube Shorts (`YouTubeShortUploader`):**
1. `POST https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable` to get an upload URI.
2. Stream the video file in chunks to the upload URI.
3. Call `setUploadProgress` after each chunk.
4. Parse the video ID from the `200 OK` body.

**Instagram Reels (`InstagramReelUploader`):**
1. `POST https://graph.instagram.com/v18.0/{ig-user-id}/media` with `media_type=REELS` and `video_url`.
2. Poll `/media?fields=status_code` until status is `FINISHED`.
3. `POST https://graph.instagram.com/v18.0/{ig-user-id}/media_publish` with `creation_id`.
4. Return the media ID.

**TikTok (`TikTokUploader`):**
1. `POST https://open.tiktokapis.com/v2/post/publish/video/init/` to initialise.
2. Upload chunks to the `upload_url` returned, calling `setUploadProgress` per chunk.
3. Return the `publish_id`.

### Implementing real OAuth token refresh

In `TokenRefreshService.callPlatformRefresh` replace the stub with real HTTP POST calls:

- **YouTube / Google:** `POST https://oauth2.googleapis.com/token` with `grant_type=refresh_token`
- **Instagram:** `GET https://graph.instagram.com/refresh_access_token` (long-lived tokens)
- **TikTok:** `POST https://open.tiktokapis.com/v2/oauth/token/refresh/`
