-- 005_create_social_accounts.sql

CREATE TABLE IF NOT EXISTS social_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL
        CHECK (platform IN ('tiktok', 'instagram', 'youtube', 'twitter')),
    platform_user_id VARCHAR(255) NOT NULL,
    username VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    profile_picture_url TEXT,
    access_token TEXT,
    refresh_token TEXT,
    token_type VARCHAR(50),
    scope TEXT,
    expires_at TIMESTAMPTZ,
    is_connected BOOLEAN NOT NULL DEFAULT TRUE,
    follower_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (user_id, platform, platform_user_id)
);

CREATE INDEX idx_social_accounts_user_id ON social_accounts (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_social_accounts_platform ON social_accounts (platform) WHERE deleted_at IS NULL;
