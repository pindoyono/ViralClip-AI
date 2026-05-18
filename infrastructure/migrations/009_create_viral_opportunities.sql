-- 009_create_viral_opportunities.sql

CREATE TABLE IF NOT EXISTS viral_opportunities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_platform VARCHAR(50) NOT NULL,
    external_video_id VARCHAR(255) NOT NULL,
    channel_id VARCHAR(255) NOT NULL,
    title TEXT NOT NULL,
    category VARCHAR(255),
    source_query VARCHAR(255),
    views BIGINT NOT NULL DEFAULT 0,
    previous_views BIGINT NOT NULL DEFAULT 0,
    likes BIGINT NOT NULL DEFAULT 0,
    comments BIGINT NOT NULL DEFAULT 0,
    subscriber_count BIGINT NOT NULL DEFAULT 0,
    published_at TIMESTAMPTZ NOT NULL,
    last_collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    view_velocity DOUBLE PRECISION NOT NULL DEFAULT 0,
    engagement_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    outlier_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    growth_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    viral_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_viral_opportunities_source_video UNIQUE (source_platform, external_video_id)
);

CREATE INDEX idx_viral_opportunities_viral_score ON viral_opportunities (viral_score DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_viral_opportunities_published_at ON viral_opportunities (published_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_viral_opportunities_category ON viral_opportunities (category) WHERE deleted_at IS NULL;

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

DROP TRIGGER IF EXISTS update_viral_opportunities_updated_at ON viral_opportunities;
CREATE TRIGGER update_viral_opportunities_updated_at
    BEFORE UPDATE ON viral_opportunities
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
