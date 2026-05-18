# ViralClip AI — REST API Reference

Base URL: `http://localhost:8080/api/v1`

All protected endpoints require an `Authorization: Bearer <access_token>` header.

Successful responses have the shape:
```json
{ "success": true, "data": { ... } }
```

Error responses:
```json
{ "success": false, "error": { "code": "error_code", "message": "Human readable message" } }
```

---

## Authentication

### Register

```
POST /auth/register
```

**Request body:**
```json
{
  "name": "Alice Smith",
  "email": "alice@example.com",
  "password": "securepassword123"
}
```

**Response `201`:**
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Alice Smith",
      "email": "alice@example.com",
      "tier": "free",
      "is_email_verified": false,
      "created_at": "2024-01-15T10:00:00Z"
    },
    "access_token": "eyJhbGci...",
    "refresh_token": "a3f5c8d2...",
    "expires_at": "2024-01-16T10:00:00Z"
  }
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice Smith","email":"alice@example.com","password":"securepassword123"}'
```

**Errors:**
- `409 Conflict` — email already registered
- `400 Bad Request` — invalid request body

---

### Login

```
POST /auth/login
```

**Request body:**
```json
{
  "email": "alice@example.com",
  "password": "securepassword123"
}
```

**Response `200`:**
```json
{
  "success": true,
  "data": {
    "user": { "id": "...", "name": "Alice Smith", "email": "alice@example.com", "tier": "free" },
    "access_token": "eyJhbGci...",
    "refresh_token": "b8e2f1...",
    "expires_at": "2024-01-16T10:00:00Z"
  }
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"securepassword123"}'
```

**Errors:**
- `401 Unauthorized` — invalid credentials

---

### Refresh Token

```
POST /auth/refresh
```

**Request body:**
```json
{ "refresh_token": "b8e2f1..." }
```

**Response `200`:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGci...",
    "expires_at": "2024-01-16T11:00:00Z"
  }
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"b8e2f1..."}'
```

---

### Logout

```
POST /auth/logout
Authorization: Bearer <token>
```

**Response `200`:**
```json
{ "success": true, "message": "Logged out successfully" }
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/logout \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Get Current User

```
GET /auth/me
Authorization: Bearer <token>
```

**Response `200`:**
```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Alice Smith",
    "email": "alice@example.com",
    "tier": "pro",
    "is_email_verified": true,
    "created_at": "2024-01-15T10:00:00Z"
  }
}
```

**curl example:**
```bash
curl http://localhost:8080/api/v1/auth/me \
  -H "Authorization: Bearer eyJhbGci..."
```

---

## Videos

### Upload Video

```
POST /videos/
Authorization: Bearer <token>
Content-Type: multipart/form-data
```

**Form fields:**
| Field               | Type    | Required | Description                        |
|---------------------|---------|----------|------------------------------------|
| `video`             | File    | Yes      | Video file (.mp4, .mov, .avi, .mkv, .webm) |
| `title`             | String  | Yes      | Video title (1–255 chars)          |
| `description`       | String  | No       | Optional description               |
| `content_profile_id`| UUID    | No       | Linked content profile             |

**Response `201`:**
```json
{
  "success": true,
  "data": {
    "id": "vid-uuid",
    "user_id": "user-uuid",
    "title": "My Podcast Episode 42",
    "status": "pending",
    "file_size": 524288000,
    "storage_url": "http://localhost/storage/videos/user-id/vid-uuid.mp4",
    "created_at": "2024-01-15T10:00:00Z"
  }
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/videos/ \
  -H "Authorization: Bearer eyJhbGci..." \
  -F "video=@/path/to/video.mp4" \
  -F "title=My Podcast Episode 42" \
  -F "description=Great interview about AI"
```

---

### List Videos

```
GET /videos/?page=1&limit=20&status=completed
Authorization: Bearer <token>
```

**Query parameters:**
| Parameter | Type    | Default | Description                            |
|-----------|---------|---------|----------------------------------------|
| `page`    | Integer | 1       | Page number                            |
| `limit`   | Integer | 20      | Items per page (max 100)               |
| `status`  | String  | (all)   | Filter: `pending`, `processing`, `completed`, `failed` |

**Response `200`:**
```json
{
  "success": true,
  "data": {
    "videos": [
      {
        "id": "vid-uuid",
        "title": "My Podcast Episode 42",
        "status": "completed",
        "duration": 3600.5,
        "clips_count": 8,
        "created_at": "2024-01-15T10:00:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "limit": 20,
      "total": 42,
      "total_pages": 3
    }
  }
}
```

**curl example:**
```bash
curl "http://localhost:8080/api/v1/videos/?page=1&limit=10&status=completed" \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Get Video

```
GET /videos/:id
Authorization: Bearer <token>
```

**curl example:**
```bash
curl http://localhost:8080/api/v1/videos/vid-uuid \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Delete Video

```
DELETE /videos/:id
Authorization: Bearer <token>
```

**Response `200`:**
```json
{ "success": true, "message": "Video deleted successfully" }
```

**Errors:**
- `400 Bad Request` — video is currently being processed
- `404 Not Found` — video not found or not owned by user

**curl example:**
```bash
curl -X DELETE http://localhost:8080/api/v1/videos/vid-uuid \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Trigger Processing

```
POST /videos/:id/process
Authorization: Bearer <token>
```

Enqueues the video for AI processing. Only works for videos in `pending` or `failed` status.

**Response `200`:**
```json
{ "success": true, "message": "Video processing started" }
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/videos/vid-uuid/process \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Get Video Clips

```
GET /videos/:videoId/clips
Authorization: Bearer <token>
```

**curl example:**
```bash
curl http://localhost:8080/api/v1/videos/vid-uuid/clips \
  -H "Authorization: Bearer eyJhbGci..."
```

---

## Clips

### List Clips

```
GET /clips/?page=1&limit=20
Authorization: Bearer <token>
```

**Response `200`:**
```json
{
  "success": true,
  "data": {
    "clips": [
      {
        "id": "clip-uuid",
        "video_id": "vid-uuid",
        "title": "Incredible Moment",
        "hook_text": "You won't believe what happens next...",
        "start_time": 120.5,
        "end_time": 165.0,
        "duration": 44.5,
        "viral_score": 0.92,
        "hashtags": ["viral","trending","wow"],
        "suggested_for": ["tiktok","reels"],
        "status": "ready"
      }
    ],
    "pagination": { "page": 1, "limit": 20, "total": 8, "total_pages": 1 }
  }
}
```

**curl example:**
```bash
curl "http://localhost:8080/api/v1/clips/?page=1" \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Get Clip

```
GET /clips/:id
Authorization: Bearer <token>
```

**curl example:**
```bash
curl http://localhost:8080/api/v1/clips/clip-uuid \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Update Clip

```
PATCH /clips/:id
Authorization: Bearer <token>
```

**Request body (all fields optional):**
```json
{
  "title": "Updated Title",
  "description": "New description",
  "hook_text": "Updated hook",
  "hashtags": ["newtag1", "newtag2"]
}
```

**curl example:**
```bash
curl -X PATCH http://localhost:8080/api/v1/clips/clip-uuid \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"title":"Updated Title","hashtags":["viral","trending"]}'
```

---

### Delete Clip

```
DELETE /clips/:id
Authorization: Bearer <token>
```

**curl example:**
```bash
curl -X DELETE http://localhost:8080/api/v1/clips/clip-uuid \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Enhance Clip Metadata (AI)

```
POST /clips/:id/metadata/enhance
Authorization: Bearer <token>
```

**Request body (optional):**
```json
{
  "platform": "tiktok",
  "niche": "tech",
  "tone": "educational"
}
```

**Response `200`:**
```json
{
  "success": true,
  "data": {
    "clip": {
      "id": "clip-uuid",
      "video_id": "vid-uuid",
      "title": "Enhanced title",
      "description": "Enhanced description",
      "hashtags": ["viral", "trending", "fyp"]
    },
    "keywords": ["viral", "shorts", "engagement"],
    "category": "Education",
    "optimal_post_times": ["7:00 PM EST on Weekdays"]
  }
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/clips/clip-uuid/metadata/enhance \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"platform":"youtube","niche":"tech","tone":"educational"}'
```

**Errors:**
- `404 Not Found` — clip not found or not owned by user
- `500 Internal Server Error` — AI service unavailable

---

## Social & Scheduling

### Connect Social Account (OAuth/token payload)

```
POST /social/connect
Authorization: Bearer <token>
```

**Request body:**
```json
{
  "platform": "tiktok",
  "username": "creator_handle",
  "access_token": "token",
  "refresh_token": "refresh_token",
  "expires_at": "2026-05-18T22:00:00Z"
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/social/connect \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"platform":"tiktok","username":"creator_handle","access_token":"token","refresh_token":"refresh_token"}'
```

---

### Disconnect Social Account

```
POST /social/disconnect
Authorization: Bearer <token>
```

**Request body:**
```json
{
  "account_id": "account-uuid"
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/social/disconnect \
  -H "Authorization: Bearer eyJhbGci..."
```

---

### Schedule a Post

```
POST /schedule
Authorization: Bearer <token>
```

**Request body:**
```json
{
  "clip_id": "clip-uuid",
  "social_account_id": "account-uuid",
  "scheduled_at": "2024-01-20T18:00:00Z",
  "publish_at": "2024-01-20T18:00:00Z",
  "caption": "Check out this amazing moment! 🔥",
  "hashtags": "#viral #trending"
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/schedule \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"clip_id":"clip-uuid","social_account_id":"acc-uuid","scheduled_at":"2024-01-20T18:00:00Z","publish_at":"2024-01-20T18:00:00Z","caption":"Amazing clip!"}'
```

---

### Publish Now

```
POST /publish
Authorization: Bearer <token>
```

**Request body:**
```json
{
  "clip_id": "clip-uuid",
  "social_account_id": "account-uuid",
  "caption": "Publish immediately",
  "hashtags": "#viral"
}
```

**curl example:**
```bash
curl -X POST http://localhost:8080/api/v1/publish \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"clip_id":"clip-uuid","social_account_id":"acc-uuid","caption":"Publish now"}'
```

---

### Publish Status

```
GET /publish/status?post_id=<post-uuid>
Authorization: Bearer <token>
```

**curl example:**
```bash
curl http://localhost:8080/api/v1/publish/status?post_id=post-uuid \
  -H "Authorization: Bearer eyJhbGci..."
```

`PublishStatus` returns:
- current post status (`scheduled`, `publishing`, `published`, `failed`, `cancelled`)
- retry count and last error message
- ordered publish attempt logs from `publishing_logs`

---

## Analytics

### Get Summary

```
GET /analytics/summary
Authorization: Bearer <token>
```

**Response `200`:**
```json
{
  "success": true,
  "data": {
    "total_views": 1250000,
    "total_likes": 45000,
    "total_comments": 3200,
    "total_shares": 8900,
    "avg_engagement_rate": 3.6,
    "published_clips": 24,
    "scheduled_posts": 6,
    "top_clip": { "id": "clip-uuid", "title": "Best Clip", "viral_score": 0.97 }
  }
}
```

**curl example:**
```bash
curl http://localhost:8080/api/v1/analytics/summary \
  -H "Authorization: Bearer eyJhbGci..."
```

---

## Trending Topics

### List Trending Topics

```
GET /trending/?platform=tiktok
Authorization: Bearer <token>
```

**Query parameters:**
| Parameter  | Type   | Description                             |
|------------|--------|-----------------------------------------|
| `platform` | String | Filter: `tiktok`, `youtube`, `instagram`|

**curl example:**
```bash
curl "http://localhost:8080/api/v1/trending/?platform=tiktok" \
  -H "Authorization: Bearer eyJhbGci..."
```

---

## AI Service Endpoints

Base URL: `http://localhost:8000/api/v1`

### Transcribe Video

```
POST /transcript
```

**Request body:**
```json
{
  "video_id": "vid-uuid",
  "storage_path": "videos/user-id/vid-uuid.mp4",
  "language": "en"
}
```

**Response `200`:**
```json
{
  "video_id": "vid-uuid",
  "language": "en",
  "duration": 3600.5,
  "full_text": "Welcome to today's episode...",
  "segments": [
    { "start": 0.0, "end": 3.2, "text": "Welcome to today's episode", "confidence": 0.98 }
  ]
}
```

---

### Generate Clip Segments

```
POST /clips
```

**Request body:**
```json
{
  "video_id": "vid-uuid",
  "storage_path": "videos/user-id/vid-uuid.mp4",
  "max_clips": 10,
  "min_duration": 15.0,
  "max_duration": 90.0,
  "content_profile": { "niche": "fitness", "tone": "motivational" }
}
```

**Response `200`:**
```json
{
  "video_id": "vid-uuid",
  "processing_time": 8.4,
  "clips": [
    {
      "start_time": 120.0,
      "end_time": 162.5,
      "duration": 42.5,
      "viral_score": 0.92,
      "hook_text": "This one tip changed everything...",
      "suggested_title": "The Secret Fitness Hack",
      "hashtags": ["fitness","health","motivation"],
      "suggested_for": ["tiktok","reels","shorts"],
      "rationale": "Strong emotional peak with clear takeaway"
    }
  ]
}
```

---

### Generate Hooks

```
POST /hooks
```

**Request body:**
```json
{
  "video_id": "vid-uuid",
  "transcript": "Full transcript text here...",
  "niche": "fitness",
  "platform": "tiktok",
  "tone": "motivational",
  "count": 5
}
```

**Response `200`:**
```json
{
  "video_id": "vid-uuid",
  "hooks": [
    {
      "text": "This changed my life in 30 days",
      "type": "statement",
      "viral_score": 0.91,
      "rationale": "Personal transformation resonates widely"
    }
  ]
}
```

---

### Generate Metadata

```
POST /metadata
```

**Request body:**
```json
{
  "video_id": "vid-uuid",
  "transcript": "Transcript text...",
  "platform": "tiktok",
  "niche": "fitness",
  "tone": "motivational"
}
```

**Response `200`:**
```json
{
  "video_id": "vid-uuid",
  "title": "30-Day Fitness Transformation",
  "description": "Learn the exact routine that transformed my body in 30 days.",
  "hashtags": ["fitness", "transformation", "workout"],
  "keywords": ["fitness tips", "30 day challenge"],
  "category": "Health & Fitness",
  "optimal_post_times": ["7:00 PM EST on Weekdays", "10:00 AM EST on Weekends"]
}
```

---

## Error Codes

| Code                | HTTP Status | Description                                |
|--------------------|-------------|--------------------------------------------|
| `bad_request`      | 400         | Invalid input or missing fields            |
| `unauthorized`     | 401         | Missing, invalid, or expired token         |
| `forbidden`        | 403         | Insufficient subscription tier             |
| `not_found`        | 404         | Resource not found or not owned by user    |
| `email_taken`      | 409         | Email address already registered           |
| `validation_error` | 422         | Field-level validation failures            |
| `internal_error`   | 500         | Unexpected server error                    |
| `insufficient_tier`| 403         | Feature requires higher subscription tier  |
