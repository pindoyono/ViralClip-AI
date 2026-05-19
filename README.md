# ViralClip AI

> Transform long-form video content into viral short-form clips using AI — automatically transcribed, segmented, scored, and ready to publish.

[![Go](https://img.shields.io/badge/Go-1.21-blue?logo=go)](https://golang.org)
[![Python](https://img.shields.io/badge/Python-3.11-blue?logo=python)](https://python.org)
[![Next.js](https://img.shields.io/badge/Next.js-14-black?logo=next.js)](https://nextjs.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## Features

- 🎬 **AI Clip Detection** — GPT-4 powered viral segment identification with engagement scoring
- 🎙️ **Speech-to-Text** — Whisper-based transcription with timestamp-accurate segments
- 🪝 **Hook Generation** — Auto-generate platform-optimized attention hooks in < 15 words
- 🔍 **Hook Detection Engine V2** — Rule-based hook detection across 5 categories (curiosity, emotion, storytelling, controversy, CTA) with 6-signal scoring: position, emphasis, pattern count, speech pauses, and repetition
- 📊 **Viral Scoring** — 0–100 AI score for each clip with rationale
- 🎞️ **Dynamic Clip Engine V2** — Profile-aware clip candidate generation using Hook×50% + Emotion×20% + Story×20% + Retention×10% composite scoring across 5 content profiles (gaming, comedy, education, politics, podcast)
- 📅 **Multi-Platform Scheduling** — Publish to TikTok, YouTube Shorts & Instagram Reels
- 📈 **Analytics Dashboard** — Real-time engagement metrics across all connected platforms
- 🔔 **Trending Topics** — Platform-wide trend monitoring for content alignment
- 🚀 **Viral Opportunity Collector** — Hourly YouTube trend ingestion with view velocity, outlier, engagement, growth, and recommendation scoring
- 🔐 **JWT Authentication** — Secure token-based auth with refresh token rotation
- 📦 **Subscription Tiers** — Free / Starter / Pro / Enterprise with Stripe integration
- 🐳 **Production-Ready** — Docker Compose + Kubernetes manifests included
- 🔁 **Dead Letter Queue** — Failed jobs persisted to DB with exponential back-off recovery via `DeadLetterWorker` + `RecoveryWorker`

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Internet / CDN                              │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                    ┌────────▼─────────┐
                    │   Nginx Proxy     │  :80 / :443
                    │  (TLS, Rate Limit)│
                    └────┬─────────┬───┘
                         │         │
             ┌───────────▼──┐  ┌───▼──────────────┐
             │  Next.js 14  │  │  Go Fiber API     │
             │  Web App     │  │  :8080            │
             │  :3000       │  │                   │
             └──────────────┘  └──────┬────────────┘
                                      │
              ┌───────────────────────┼─────────────────────┐
              │                       │                       │
    ┌─────────▼────────┐  ┌───────────▼──────┐  ┌──────────▼───────┐
    │  PostgreSQL 16   │  │  Redis 7          │  │  Python FastAPI  │
    │  (Primary Store) │  │  (Cache / Queue)  │  │  AI Service      │
    └──────────────────┘  └───────────────────┘  │  :8000           │
                                    │             └──────────────────┘
                          ┌─────────▼────────┐         │
                          │  Go Worker       │─────────►│
                          │  (Background     │  Whisper + GPT-4
                          │   Processing)    │
                          └──────────────────┘
```

### Data Flow

```
User uploads video
      │
      ▼
API stores video record (status: pending)
      │
      ▼
Worker polls pending videos → calls AI Service
      │
      ▼
AI Service:
  1. Extracts audio (FFmpeg)
  2. Transcribes speech (Whisper)
  3. Identifies viral segments (GPT-4)
  4. Generates hooks & metadata
      │
      ▼
Clips created in DB (status: ready)
      │
      ▼
User schedules clips → Worker publishes to platforms
      │
      ▼
Analytics synced back to dashboard
```

---

## Tech Stack

| Layer          | Technology               | Purpose                            |
|----------------|-------------------------|------------------------------------|
| Frontend       | Next.js 14 (App Router) | React UI with SSR                  |
| State          | Zustand + React Query    | Client state + server data cache   |
| API            | Go 1.21 + Fiber v2       | High-performance REST API          |
| AI Service     | Python 3.11 + FastAPI    | ML inference & AI integrations     |
| Worker         | Go 1.21                  | Background job processing          |
| Database       | PostgreSQL 16            | Primary data store                 |
| Cache / Queue  | Redis 7                  | Caching & job queuing              |
| Transcription  | OpenAI Whisper           | Speech-to-text                     |
| AI             | OpenAI GPT-4             | Content analysis & generation      |
| Auth           | JWT (HS256)              | Stateless authentication           |
| Payments       | Stripe                   | Subscription management            |
| Reverse Proxy  | Nginx                    | TLS termination, rate limiting     |
| Container      | Docker + Docker Compose  | Development & deployment           |
| Orchestration  | Kubernetes (Helm)        | Production scaling                 |

---

## Prerequisites

| Tool            | Version  | Install                                          |
|-----------------|----------|--------------------------------------------------|
| Docker          | ≥ 24.0   | https://docs.docker.com/get-docker/              |
| Docker Compose  | ≥ 2.20   | Included with Docker Desktop                     |
| Go              | ≥ 1.21   | https://go.dev/dl/ (for local dev)               |
| Python          | ≥ 3.11   | https://python.org (for local dev)               |
| Node.js         | ≥ 20     | https://nodejs.org (for local dev)               |
| pnpm            | ≥ 8      | `npm i -g pnpm` (for local dev)                  |
| FFmpeg          | ≥ 6      | https://ffmpeg.org/download.html                 |

---

## Quick Start

Get the full stack running in 5 steps:

```bash
# 1. Clone the repository
git clone https://github.com/pindoyono/ViralClip-AI.git
cd ViralClip-AI

# 2. Copy and configure environment variables
cp .env.example .env
# Edit .env and set at minimum: OPENAI_API_KEY, JWT_SECRET

# 3. Start all services with Docker Compose
docker compose up -d

# 4. Wait for services to be healthy (≈ 60s)
docker compose ps

# 5. Open the application
open http://localhost:3000
```

The following ports will be available:

| Service      | URL                          |
|-------------|------------------------------|
| Web App     | http://localhost:3000         |
| API         | http://localhost:8080         |
| AI Service  | http://localhost:8000         |
| PostgreSQL  | localhost:5432                |
| Redis       | localhost:6379                |

---

## Development Setup

### Environment Variables

Copy `.env.example` to `.env` and fill in the required values:

```bash
cp .env.example .env
```

#### Required Variables

| Variable           | Description                          | Example                        |
|-------------------|--------------------------------------|--------------------------------|
| `OPENAI_API_KEY`  | OpenAI API key for GPT-4 & Whisper  | `sk-...`                      |
| `JWT_SECRET`      | HS256 signing secret (≥ 32 chars)   | `your-super-secret-key`        |
| `DATABASE_PASSWORD` | PostgreSQL password               | `strongpassword123`            |

#### Optional Variables

| Variable                   | Default                        | Description                      |
|---------------------------|--------------------------------|----------------------------------|
| `APP_ENV`                 | `development`                  | Application environment          |
| `API_PORT`                | `8080`                         | Go API server port               |
| `AI_SERVICE_PORT`         | `8000`                         | Python AI service port           |
| `DATABASE_NAME`           | `viralclip`                    | PostgreSQL database name         |
| `DATABASE_USER`           | `viralclip`                    | PostgreSQL user                  |
| `DATABASE_PORT`           | `5432`                         | PostgreSQL port                  |
| `REDIS_URL`               | `redis://localhost:6379/0`     | Redis connection URL             |
| `REDIS_PASSWORD`          | *(empty)*                      | Redis password                   |
| `JWT_EXPIRES_IN`          | `24h`                          | Access token lifetime            |
| `JWT_REFRESH_EXPIRES_IN`  | `168h`                         | Refresh token lifetime           |
| `WHISPER_MODEL`           | `base`                         | Whisper model size               |
| `WHISPER_DEVICE`          | `cpu`                          | `cpu` or `cuda`                  |
| `OPENAI_MODEL`            | `gpt-4-turbo-preview`          | OpenAI chat model                |
| `STORAGE_PROVIDER`        | `local`                        | `local` or `s3`                  |
| `LOCAL_STORAGE_PATH`      | `./storage`                    | Local file storage path          |
| `CORS_ORIGINS`            | `http://localhost:3000`        | Allowed CORS origins             |
| `LOG_LEVEL`               | `info`                         | `debug`, `info`, `warn`, `error` |
| `WORKER_CONCURRENCY`      | `4`                            | Worker goroutine count           |
| `NEXT_PUBLIC_API_URL`     | `http://localhost:8080`        | API URL exposed to browser       |
| `STRIPE_SECRET_KEY`       | *(optional)*                   | Stripe secret key                |
| `GOOGLE_CLIENT_ID`        | *(optional)*                   | Google OAuth client ID           |
| `SENTRY_DSN`              | *(optional)*                   | Sentry error tracking DSN        |
| `YOUTUBE_REGION_CODE`     | `US`                           | Region used for YouTube trend collection |
| `YOUTUBE_MAX_RESULTS`     | `10`                           | Videos fetched per trend query   |
| `YOUTUBE_LOOKBACK_WINDOW` | `168h`                         | Recency window for collected videos |

### Running Individual Services

#### Go API

```bash
cd apps/api
go mod download
cp .env.example .env  # configure database/redis/jwt
go run main.go
```

#### Go Worker

```bash
cd apps/worker
go mod download
go run main.go
```

#### Python AI Service

```bash
cd apps/ai-service
python -m venv .venv
source .venv/bin/activate        # Windows: .venv\Scripts\activate
pip install -r requirements.txt
cp .env.example .env             # configure openai key
uvicorn main:app --reload --port 8000
```

#### Next.js Web App

```bash
cd apps/web
pnpm install
cp .env.example .env.local       # configure API URL
pnpm dev
```

---

## Dead Letter Queue + Recovery Worker

### Architecture Decisions

- **Existing DLQ mechanism** in `apps/worker/internal/queue/queue.go` pushes failed jobs to explicit Redis lists such as `transcript_dlq`, `clip_dlq`, `upload_dlq`, and `analytics_dlq` via `PushDead()`. Task 1 builds the persistence and recovery layer on top of this.
- **DeadLetterWorker** (`apps/worker/internal/workers/dlq_worker.go`) spawns one consumer goroutine per monitored DLQ. It BLPOP-s dead jobs, serialises them and writes a `FailedJobRecord` to the `failed_jobs` table. Separation from `RecoveryWorker` keeps concerns clear and allows dead-letter inspection before any recovery attempt.
- **RecoveryWorker** (`apps/worker/internal/workers/recovery_worker.go`) polls the `failed_jobs` table every 30 s. It uses **exponential back-off** (`2^retryCount × 30 s`, capped at 1 hour) so transient failures don't hammer the AI service. The worker-level `maxRetries` setting acts as a hard cap independent of any per-job value.
- **QueueMetricsService** (`apps/api/internal/services/queue_metrics.go`) queries Redis LLEN for all queue and DLQ depths and counts `FailedJob` rows by status — no additional infrastructure required.
- All error details are stored in the job's `Metadata["error"]` field before the `PushDead` call so the DeadLetterWorker can extract them without a round-trip.

### API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/queue/status` | Yes | Live queue depths (Redis) + failed job aggregates (DB) |
| `GET` | `/api/v1/queue/failed` | Yes | Paginated list of failed jobs; filterable by `queue` and `status` |
| `GET` | `/api/v1/queue/retry` | Yes | Jobs currently eligible for recovery (pending/recovering) |

### Directory Changes

- `infrastructure/migrations/011_create_failed_jobs.sql` — `failed_jobs` table and indexes
- `apps/worker/internal/workers/dlq_worker.go` — `DeadLetterWorker`
- `apps/worker/internal/workers/dlq_worker_test.go` — DLQ worker unit tests
- `apps/worker/internal/workers/recovery_worker.go` — `RecoveryWorker` with exponential back-off
- `apps/worker/internal/workers/recovery_worker_test.go` — recovery worker unit tests
- `apps/api/internal/models/models.go` — `FailedJob` model added
- `apps/api/internal/services/queue_metrics.go` — `QueueMetricsService`
- `apps/api/internal/services/queue_metrics_test.go` — metrics service tests
- `apps/api/internal/handlers/queue.go` — `QueueHandler`
- `apps/api/internal/handlers/queue_test.go` — handler integration tests

### FailedJob Lifecycle

```
Job fails in worker
      │
      ▼
handleJobFailure stores error in Metadata, calls PushDead
      │
      ▼
Explicit DLQ Redis list (`transcript_dlq`, `clip_dlq`, `subtitle_dlq`, `upload_dlq`, `analytics_dlq`)
      │
      ▼
DeadLetterWorker reads DLQ → writes FailedJobRecord (status: pending)
      │
      ▼
RecoveryWorker polls every 30 s
  ├─ retry_count < max_retries → push back to original queue (status: recovering)
  └─ retry_count >= max_retries → mark exhausted
```

### Migration

Apply `infrastructure/migrations/011_create_failed_jobs.sql` before running the updated worker or API in production.

---

## Viral Opportunity Collector

### Architecture Decisions

- **YouTubeCollector (`apps/worker/internal/trends/collector.go`)** calls the YouTube Data API in two stages: search for recent high-view candidates, then hydrate video and channel statistics before persistence.
- **TrendEngine (`apps/worker/internal/trends/engine.go`)** calculates the required metrics for every collected record: `view_velocity`, `engagement_rate`, `outlier_score`, `growth_score`, and `viral_score`.
- **TrendCollectorWorker (`apps/worker/internal/trends/worker.go`)** runs immediately on worker startup and every hour after that, derives search seeds from `content_profiles`, and upserts rows into `viral_opportunities`.
- **RecommendationEngine (`apps/api/internal/services/viral_opportunities.go`)** matches stored opportunities against each user's content profile niche and keywords so recommendations stay personalized without changing the existing auth or API architecture.
- **API delivery** stays inside the existing Fiber + GORM stack with three new authenticated endpoints under `/api/v1/viral-opportunities`.

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/viral-opportunities` | Paginated list of collected opportunities with optional `category` and `query` filters |
| `GET` | `/api/v1/viral-opportunities/trending` | Recent top-ranked opportunities from the last 72 hours |
| `GET` | `/api/v1/viral-opportunities/recommendations` | Personalized recommendations ranked against the authenticated user's content profiles |

### Directory Changes

- `apps/api/internal/handlers/viral_opportunities.go` — new API handler
- `apps/api/internal/services/viral_opportunities.go` — list/trending service + recommendation engine
- `apps/api/internal/handlers/viral_opportunities_test.go` — API integration coverage
- `apps/api/internal/services/viral_opportunities_test.go` — recommendation and trending unit coverage
- `apps/worker/internal/trends/collector.go` — YouTubeCollector implementation
- `apps/worker/internal/trends/engine.go` — TrendEngine scoring logic
- `apps/worker/internal/trends/worker.go` — hourly TrendCollectorWorker persistence flow
- `apps/worker/internal/trends/*_test.go` — collector, scoring, and worker tests
- `infrastructure/migrations/009_create_viral_opportunities.sql` — schema migration for collected opportunities

### Migration

Apply `infrastructure/migrations/009_create_viral_opportunities.sql` to create the `viral_opportunities` table and supporting indexes before running the worker in production.

---

## Running Tests

### Go Tests (API & Worker)

```bash
# API tests
cd apps/api
go test ./...

# Worker tests
cd apps/worker
go test ./...

# With coverage
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

### Python Tests (AI Service)

```bash
cd apps/ai-service
pip install pytest pytest-asyncio
pytest tests/ -v
```

### TypeScript Tests (Web App)

```bash
cd apps/web
pnpm add -D jest @swc/jest @testing-library/react @testing-library/jest-dom identity-obj-proxy
pnpm jest
```

---

## API Endpoints

### Authentication

| Method | Endpoint                     | Auth | Description              |
|--------|------------------------------|------|--------------------------|
| POST   | `/api/v1/auth/register`      | No   | Register new user        |
| POST   | `/api/v1/auth/login`         | No   | Login and get tokens     |
| POST   | `/api/v1/auth/logout`        | Yes  | Invalidate refresh token |
| POST   | `/api/v1/auth/refresh`       | No   | Refresh access token     |
| GET    | `/api/v1/auth/me`            | Yes  | Get current user         |
| POST   | `/api/v1/auth/forgot-password` | No | Request password reset   |
| POST   | `/api/v1/auth/reset-password`  | No | Reset password           |

### Videos

| Method | Endpoint                             | Auth | Description                      |
|--------|--------------------------------------|------|----------------------------------|
| POST   | `/api/v1/videos/`                    | Yes  | Upload video (multipart)         |
| GET    | `/api/v1/videos/`                    | Yes  | List user videos                 |
| GET    | `/api/v1/videos/:id`                 | Yes  | Get video by ID                  |
| DELETE | `/api/v1/videos/:id`                 | Yes  | Delete video                     |
| POST   | `/api/v1/videos/:id/process`         | Yes  | Trigger AI processing            |
| GET    | `/api/v1/videos/:id/clips`           | Yes  | List clips for video             |
| POST   | `/api/v1/videos/:id/hooks/detect`    | Yes  | **[V2]** Detect hook moments     |
| GET    | `/api/v1/videos/:id/hooks`           | Yes  | **[V2]** List stored hook detections |
| POST   | `/api/v1/videos/:id/clips/v2/generate` | Yes | **[V2]** Generate clips (profile-aware) |

### Clips

| Method | Endpoint                     | Auth | Description              |
|--------|------------------------------|------|--------------------------|
| GET    | `/api/v1/clips/`             | Yes  | List all user clips      |
| GET    | `/api/v1/clips/:id`          | Yes  | Get clip by ID           |
| PATCH  | `/api/v1/clips/:id`          | Yes  | Update clip metadata     |
| DELETE | `/api/v1/clips/:id`          | Yes  | Delete clip              |

### Social & Scheduling

| Method | Endpoint                          | Auth | Description              |
|--------|-----------------------------------|------|--------------------------|
| GET    | `/api/v1/social/accounts`         | Yes  | List connected accounts  |
| DELETE | `/api/v1/social/accounts/:id`     | Yes  | Disconnect account       |
| POST   | `/api/v1/social/schedule`         | Yes  | Schedule clip for post   |
| GET    | `/api/v1/social/schedule`         | Yes  | List scheduled posts     |
| DELETE | `/api/v1/social/schedule/:id`     | Yes  | Cancel scheduled post    |

### Analytics & Trending

| Method | Endpoint                     | Auth | Description              |
|--------|------------------------------|------|--------------------------|
| GET    | `/api/v1/analytics/summary`  | Yes  | Get analytics summary    |
| GET    | `/api/v1/trending/`          | Yes  | Get trending topics      |

### AI Service

| Method | Endpoint                       | Description                             |
|--------|--------------------------------|-----------------------------------------|
| POST   | `/api/v1/transcript`           | Transcribe video with Whisper           |
| POST   | `/api/v1/clips`                | Identify viral clip segments (GPT-4)    |
| POST   | `/api/v1/clips/v2/generate`    | **[V2]** Profile-aware clip generation  |
| POST   | `/api/v1/hooks`                | Generate viral hooks (GPT-4)            |
| POST   | `/api/v1/hooks/v2/detect`      | **[V2]** Detect hook moments in transcript |
| POST   | `/api/v1/metadata`             | Generate platform metadata              |
| GET    | `/health`                      | Health check                            |

---

## Database Schema

| Table              | Description                                         |
|--------------------|-----------------------------------------------------|
| `users`            | User accounts, auth, subscription tier              |
| `content_profiles` | AI content strategy per user/platform               |
| `videos`           | Uploaded source videos with processing state        |
| `clips`            | AI-generated viral clip segments                    |
| `social_accounts`  | Connected TikTok/YouTube/Instagram accounts         |
| `scheduled_posts`  | Clips queued for social media publishing            |
| `clip_analytics`   | Per-clip engagement metrics by platform             |
| `trending_topics`  | Platform-wide trending content                      |
| `hook_detections`  | **[V2]** Hook moments detected in video transcripts |
| `failed_jobs`      | Dead-letter queue entries persisted for recovery    |

---

## Deployment

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for complete deployment instructions.

### Quick Production Deployment (Docker Compose)

```bash
# Set production environment variables
export JWT_SECRET="$(openssl rand -hex 32)"
export DATABASE_PASSWORD="$(openssl rand -hex 24)"
export OPENAI_API_KEY="sk-..."

docker compose -f docker-compose.yml up -d
```

### Kubernetes

```bash
# Apply manifests
kubectl apply -f infrastructure/kubernetes/
```

---

## Contributing

1. Fork the repository
2. Create your feature branch: `git checkout -b feat/amazing-feature`
3. Commit your changes: `git commit -m 'feat: add amazing feature'`
4. Push to the branch: `git push origin feat/amazing-feature`
5. Open a Pull Request

Please follow [Conventional Commits](https://www.conventionalcommits.org/) for commit messages.

### Code Style

- **Go**: `gofmt` + `golangci-lint`
- **Python**: `black` + `ruff`
- **TypeScript**: `eslint` + `prettier`

---

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
