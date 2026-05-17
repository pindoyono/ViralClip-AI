-- 002_create_content_profiles.sql

CREATE TABLE IF NOT EXISTS content_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    niche VARCHAR(255),
    tone VARCHAR(100),
    target_audience TEXT,
    platforms TEXT[] DEFAULT '{}',
    default_hashtags TEXT[] DEFAULT '{}',
    auto_generate_clips BOOLEAN NOT NULL DEFAULT TRUE,
    auto_add_subtitles BOOLEAN NOT NULL DEFAULT TRUE,
    clip_min_duration INTEGER NOT NULL DEFAULT 15,
    clip_max_duration INTEGER NOT NULL DEFAULT 90,
    max_clips_per_video INTEGER NOT NULL DEFAULT 10,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_content_profiles_user_id ON content_profiles (user_id) WHERE deleted_at IS NULL;
