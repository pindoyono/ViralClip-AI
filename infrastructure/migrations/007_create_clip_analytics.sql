-- 007_create_clip_analytics.sql

CREATE TABLE IF NOT EXISTS clip_analytics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    clip_id UUID NOT NULL REFERENCES clips (id) ON DELETE CASCADE,
    scheduled_post_id UUID REFERENCES scheduled_posts (id) ON DELETE SET NULL,
    platform VARCHAR(50) NOT NULL,
    views BIGINT NOT NULL DEFAULT 0,
    likes BIGINT NOT NULL DEFAULT 0,
    comments BIGINT NOT NULL DEFAULT 0,
    shares BIGINT NOT NULL DEFAULT 0,
    saves BIGINT NOT NULL DEFAULT 0,
    reach BIGINT NOT NULL DEFAULT 0,
    impressions BIGINT NOT NULL DEFAULT 0,
    engagement_rate FLOAT DEFAULT 0.0,
    play_rate FLOAT DEFAULT 0.0,
    avg_watch_time FLOAT DEFAULT 0.0,
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_clip_analytics_clip_id ON clip_analytics (clip_id);
CREATE INDEX idx_clip_analytics_platform ON clip_analytics (platform);
CREATE INDEX idx_clip_analytics_synced_at ON clip_analytics (synced_at DESC);
