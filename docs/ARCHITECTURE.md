# ViralClip AI — Architecture Deep Dive

## Overview

ViralClip AI is a microservices-based SaaS platform consisting of four primary services communicating via HTTP APIs and a shared PostgreSQL database. The design prioritizes horizontal scalability, clear service boundaries, and resilience through asynchronous processing.

---

## Service Responsibilities

### 1. Go Fiber API (`apps/api`)

The primary HTTP gateway for all client requests.

**Responsibilities:**
- User registration, authentication (JWT), and session management
- Video upload and metadata management
- Clip CRUD operations
- Social account management and post scheduling
- Analytics aggregation queries
- WebSocket connections for real-time status updates

**Key design decisions:**
- Uses `gorm` with soft deletes for all user-generated content
- JWT access tokens (15-minute TTL) + opaque refresh tokens (7-day TTL)
- All handlers receive `*gorm.DB` directly (no repository abstraction layer) for simplicity at this scale
- Fiber's zero-allocation router provides sub-millisecond routing latency

**Package structure:**
```
apps/api/internal/
├── config/       Configuration loading and validation
├── dto/          Request/response data transfer objects
├── handlers/     HTTP handlers (auth, video, clip, social, analytics)
├── middleware/   JWT auth, request logging, CORS
├── models/       GORM models and database schema
├── repositories/ (future: query abstraction)
├── routes/       Route registration
├── server/       Server lifecycle management
├── services/     Business logic layer
├── utils/        JWT, hashing, HTTP response helpers
└── websocket/    Real-time update broadcasting
```

### 2. Python FastAPI AI Service (`apps/ai-service`)

Handles all compute-heavy ML tasks in isolation.

**Responsibilities:**
- Video transcription via OpenAI Whisper
- Viral segment identification via GPT-4
- Hook and metadata generation
- Video/subtitle FFmpeg processing
- Content categorization

**Key design decisions:**
- Separated from Go API to allow independent scaling (GPU instances for AI)
- Lazy model loading: Whisper model loaded once on first request and cached globally
- All OpenAI clients use a global singleton to reuse connection pools
- FastAPI's async handlers enable concurrent request processing
- Path traversal protection on all file operations via `_validate_storage_path`

**Package structure:**
```
apps/ai-service/app/
├── config.py           Pydantic settings with env var support
├── models/schemas.py   Request/response Pydantic models
├── routers/            FastAPI route handlers (one file per resource)
├── services/           Business logic (transcript, clip, hook, metadata)
└── utils/              FFmpeg utilities, file helpers
```

### 3. Go Worker (`apps/worker`)

Background job processor for async operations.

**Responsibilities:**
- Polling for pending videos and dispatching to AI service
- Publishing scheduled social media posts
- Periodic cleanup of soft-deleted records (30-day retention)
- Analytics synchronization from social platform APIs

**Key design decisions:**
- Poll-based architecture (no message broker dependency for v1)
- Batch size limited to 10 videos / 20 posts per poll cycle to prevent overload
- Context-aware processing respects graceful shutdown signals
- Workers run concurrently via `go` goroutines with shared DB connection pool

**Worker types:**
```
VideoProcessingWorker  → Calls AI Service → updates video/clip status
PublishingWorker       → Posts to social platform APIs → updates post status
CleanupWorker          → Hard-deletes soft-deleted records > 30 days old
AnalyticsWorker        → Syncs engagement metrics from social platforms
```

### 4. Next.js Web App (`apps/web`)

React-based SPA with SSR capabilities.

**Responsibilities:**
- Authentication flows (register, login, OAuth)
- Video upload with progress tracking
- Clip review and editing interface
- Social account connection and post scheduling
- Analytics dashboard with charts

**Key design decisions:**
- App Router for file-based routing with server components
- Zustand for lightweight client state (auth store)
- React Query (TanStack Query) for server state, caching, and background refetching
- Axios interceptor auto-attaches JWT and handles 401 → refresh token rotation
- Tailwind CSS for utility-first styling

---

## Data Model

```
users
  ├── content_profiles (1:N)
  ├── videos (1:N)
  │    └── clips (1:N)
  │         ├── clip_analytics (1:N)
  │         └── scheduled_posts (1:N)
  └── social_accounts (1:N)

trending_topics (standalone, populated by AnalyticsWorker)
```

### Key Relationships

- A **User** can have multiple **ContentProfiles** (one per platform/niche strategy)
- A **Video** is optionally linked to a **ContentProfile** to personalize AI output
- A **Clip** is always linked to its parent **Video** and **User**
- A **ScheduledPost** links a **Clip** to a specific **SocialAccount** with a publish time
- **ClipAnalytics** records are appended each time analytics are synced (time-series)

---

## Authentication Flow

```
Client                API                 DB
  │                    │                   │
  ├─ POST /auth/login ─►                   │
  │                    ├─ lookup user ─────►
  │                    ◄── user record ────┤
  │                    │ verify bcrypt      │
  │                    ├─ generate JWT      │
  │                    ├─ generate refresh  │
  │                    ├─ store refresh ───►│
  ◄── JWT + refresh ───┤                   │
  │                    │                   │
  │  (15 min later)    │                   │
  ├─ POST /auth/refresh►                   │
  │  {refresh_token}   ├─ lookup refresh ──►
  │                    ◄── user record ────┤
  │                    ├─ generate new JWT  │
  ◄── new JWT ─────────┤                   │
```

**Token security properties:**
- Access tokens: HS256 signed, 15-minute TTL, contain `user_id`, `email`, `tier`
- Refresh tokens: 32-byte cryptographically random hex, stored hashed in DB
- Logout invalidates refresh token immediately (prevents reuse)
- Each login/refresh rotates the refresh token

---

## Video Processing Pipeline

```
┌──────────────────────────────────────────────────────────────────┐
│                    Full Processing Flow                           │
└──────────────────────────────────────────────────────────────────┘

1. User POSTs video file
   → API saves to local/S3 storage
   → Creates Video record (status: pending)
   → Returns video ID immediately

2. User POSTs to /videos/:id/process
   → API sets status: processing

3. VideoProcessingWorker (polling every 30s)
   → Finds videos with status: processing
   → Calls AI Service POST /process/video

4. AI Service processes asynchronously:
   a. FFmpeg extracts audio → WAV file
   b. Whisper transcribes → segments with timestamps
   c. GPT-4 analyzes segments → identifies top viral moments
   d. For each clip: generates title, hook, hashtags, viral score

5. Worker receives response
   → Creates Clip records in DB (status: ready)
   → Updates Video (status: completed, processed_at: now)

6. WebSocket broadcasts update to connected client
   → Frontend updates UI without polling

Total time: 2–10 minutes depending on video length and Whisper model
```

---

## Scalability Considerations

### Horizontal Scaling

All services are stateless and can scale horizontally:
- **API**: Multiple instances behind Nginx load balancer
- **Worker**: Multiple instances, each polls DB independently (idempotent)
- **AI Service**: Scale to GPU instances for production Whisper inference

### Database Scaling

- Read replicas for analytics queries (future)
- Connection pooling via pgx (max 25 connections per API instance)
- GORM soft deletes + cleanup worker prevents unbounded table growth

### Caching Strategy

- Redis caches trending topics (TTL: 1 hour)
- React Query caches API responses client-side (stale-while-revalidate)
- Whisper model cached in Python process memory after first load

### Rate Limiting (Nginx)

```nginx
limit_req_zone $binary_remote_addr zone=api:10m rate=100r/m;
limit_req_zone $binary_remote_addr zone=upload:10m rate=10r/m;
```

---

## Security Model

| Concern                  | Mitigation                                          |
|--------------------------|-----------------------------------------------------|
| Authentication           | JWT with short TTL + refresh token rotation         |
| SQL Injection            | GORM parameterized queries throughout               |
| Path Traversal           | `_validate_storage_path()` in AI service            |
| CSRF                     | SameSite cookie + Authorization header              |
| Secrets in code          | All secrets via environment variables               |
| Password storage         | bcrypt with cost factor 12                          |
| Token secrets            | Minimum 32-byte secrets enforced                   |
| Rate limiting            | Nginx rate limits per IP                            |
| Input validation         | Pydantic (Python) + BodyParser validation (Go)      |
| Subscription enforcement | `RequireTier` middleware on protected endpoints     |

---

## Monitoring & Observability

- **Structured logging**: zerolog (Go) / loguru (Python) — JSON in production
- **Health checks**: `/health` endpoints on all services
- **Prometheus metrics**: `/metrics` endpoint on API (via fiber middleware)
- **Error tracking**: Sentry SDK in AI service
- **Request tracing**: Correlation IDs propagated through service calls (future)
