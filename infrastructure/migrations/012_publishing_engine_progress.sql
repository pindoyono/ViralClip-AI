-- 012_publishing_engine_progress.sql
-- Adds upload_progress tracking to scheduled_posts and token_refresh_attempts
-- to social_accounts for the TokenRefreshService.

-- Track per-post upload progress (0–100) so the API can surface it to clients.
ALTER TABLE scheduled_posts
    ADD COLUMN IF NOT EXISTS upload_progress INTEGER NOT NULL DEFAULT 0
        CHECK (upload_progress >= 0 AND upload_progress <= 100);

-- Track the number of consecutive token-refresh failures per social account.
-- The TokenRefreshService increments this counter on each failure and resets it
-- to 0 on success.  Operators can use this to identify accounts that need
-- re-authorisation.
ALTER TABLE social_accounts
    ADD COLUMN IF NOT EXISTS token_refresh_attempts INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_social_accounts_expiring_tokens
    ON social_accounts (expires_at)
    WHERE is_active = TRUE AND deleted_at IS NULL;
