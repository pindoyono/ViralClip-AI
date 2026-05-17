-- 003_create_videos.sql

CREATE TABLE IF NOT EXISTS videos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    content_profile_id UUID REFERENCES content_profiles (id) ON DELETE SET NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    original_filename VARCHAR(500) NOT NULL,
    storage_path TEXT NOT NULL,
    thumbnail_url TEXT,
    duration FLOAT,
    file_size BIGINT,
    width INTEGER,
    height INTEGER,
    fps FLOAT,
    resolution VARCHAR(20),
    video_codec VARCHAR(50),
    audio_codec VARCHAR(50),
    status VARCHAR(50) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    transcript TEXT,
    language VARCHAR(10),
    error_message TEXT,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_videos_user_id ON videos (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_videos_status ON videos (status) WHERE deleted_at IS NULL;
CREATE INDEX idx_videos_created_at ON videos (created_at DESC) WHERE deleted_at IS NULL;
