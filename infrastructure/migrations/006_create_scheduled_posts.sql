-- 006_create_scheduled_posts.sql

CREATE TABLE IF NOT EXISTS scheduled_posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    clip_id UUID NOT NULL REFERENCES clips (id) ON DELETE CASCADE,
    social_account_id UUID NOT NULL REFERENCES social_accounts (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL,
    caption TEXT,
    hashtags TEXT[] DEFAULT '{}',
    scheduled_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'published', 'failed', 'cancelled')),
    platform_post_id VARCHAR(255),
    platform_post_url TEXT,
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_scheduled_posts_user_id ON scheduled_posts (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_scheduled_posts_status ON scheduled_posts (status, scheduled_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_scheduled_posts_clip_id ON scheduled_posts (clip_id) WHERE deleted_at IS NULL;
