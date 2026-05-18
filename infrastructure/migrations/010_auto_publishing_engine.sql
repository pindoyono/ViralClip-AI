-- 010_auto_publishing_engine.sql

-- Ensure scheduled_posts has publish_at for scheduler/publisher coordination.
ALTER TABLE scheduled_posts
    ADD COLUMN IF NOT EXISTS publish_at TIMESTAMPTZ;

UPDATE scheduled_posts
SET publish_at = scheduled_at
WHERE publish_at IS NULL;

-- Expand scheduled_posts status lifecycle for scheduler + publishing workers.
ALTER TABLE scheduled_posts
    DROP CONSTRAINT IF EXISTS scheduled_posts_status_check;

ALTER TABLE scheduled_posts
    ADD CONSTRAINT scheduled_posts_status_check
        CHECK (status IN ('pending', 'scheduled', 'publishing', 'published', 'failed', 'cancelled'));

ALTER TABLE scheduled_posts
    ALTER COLUMN status SET DEFAULT 'scheduled';

CREATE INDEX IF NOT EXISTS idx_scheduled_posts_publish_at
    ON scheduled_posts (publish_at)
    WHERE deleted_at IS NULL;

-- Ensure token expiry column exists and is canonical on social accounts.
ALTER TABLE social_accounts
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- Publish attempt/audit logs.
CREATE TABLE IF NOT EXISTS publishing_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES scheduled_posts (id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL,
    message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_publishing_logs_post_id
    ON publishing_logs (post_id)
    WHERE deleted_at IS NULL;

