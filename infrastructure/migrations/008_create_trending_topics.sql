-- 008_create_trending_topics.sql

CREATE TABLE IF NOT EXISTS trending_topics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform VARCHAR(50) NOT NULL,
    topic VARCHAR(255) NOT NULL,
    hashtag VARCHAR(255),
    category VARCHAR(100),
    trend_score FLOAT DEFAULT 0.0,
    view_count BIGINT DEFAULT 0,
    post_count BIGINT DEFAULT 0,
    region VARCHAR(10) DEFAULT 'global',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    trending_since TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_trending_topics_platform ON trending_topics (platform) WHERE is_active = TRUE;
CREATE INDEX idx_trending_topics_trend_score ON trending_topics (trend_score DESC) WHERE is_active = TRUE;
CREATE INDEX idx_trending_topics_category ON trending_topics (category) WHERE is_active = TRUE;

-- Function to automatically update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to all main tables
DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['users', 'content_profiles', 'videos', 'clips', 'social_accounts', 'scheduled_posts', 'clip_analytics', 'trending_topics']
    LOOP
        EXECUTE format(
            'CREATE TRIGGER update_%s_updated_at BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION update_updated_at_column()',
            t, t
        );
    END LOOP;
END;
$$;
