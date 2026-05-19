# Learning Feedback Engine

This document describes Task 3 learning-loop components in `apps/api`.

## Components

### 1) LearningEngine
- File: `apps/api/internal/services/learning_engine.go`
- Responsibilities:
  - Calculate CPS (Content Performance Score)
  - Rank top/worst clips
  - Generate profile recommendations
  - Delegate hook pattern analysis to `PatternAnalyzer`

### 2) PatternAnalyzer
- File: `apps/api/internal/services/pattern_analyzer.go`
- Responsibilities:
  - Aggregate hook-level records
  - Compute average CPS per hook type
  - Compute `%` improvement over baseline
  - Return ranked hook patterns

### 3) HookPerformanceTracker
- File: `apps/api/internal/services/hook_performance_tracker.go`
- Responsibilities:
  - Collect learning-loop metrics from `hook_detections`, `clips`, and `clip_analytics`
  - Return records containing:
    - `views`
    - `watch_time`
    - `retention`
    - `likes`
    - `comments`
    - `ctr`
    - `subscriber_gain`
  - Compute per-record CPS inputs

## CPS Formula

```
CPS = (WatchTime × 30%)
    + (Retention × 25%)
    + (Engagement × 20%)
    + (CTR × 15%)
    + (SubscriberGain × 10%)
```

Implementation details:
- CPS is normalized to `0..100`.
- Engagement is derived from interactions over views.
- Retention is derived from `watch_time / duration`.

## API Endpoints

All routes are under `/api/v1/analytics`:

- `GET /top-clips`
- `GET /worst-clips`
- `GET /hook-patterns`
- `GET /recommendations`

These are registered in `apps/api/internal/routes/routes.go` and handled by `AnalyticsHandler`.
