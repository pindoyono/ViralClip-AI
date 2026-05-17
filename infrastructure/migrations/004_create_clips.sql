-- 004_create_clips.sql

CREATE TABLE IF NOT EXISTS clips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    storage_path TEXT NOT NULL,
    thumbnail_url TEXT,
    duration FLOAT,
    start_time FLOAT NOT NULL,
    end_time FLOAT NOT NULL,
    viral_score FLOAT DEFAULT 0.0,
    hashtags TEXT[] DEFAULT '{}',
    suggested_platforms TEXT[] DEFAULT '{}',
    hook_text TEXT,
    rationale TEXT,
    has_subtitles BOOLEAN NOT NULL DEFAULT FALSE,
    subtitle_path TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'ready', 'failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_clips_video_id ON clips (video_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_clips_user_id ON clips (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_clips_viral_score ON clips (viral_score DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_clips_status ON clips (status) WHERE deleted_at IS NULL;
