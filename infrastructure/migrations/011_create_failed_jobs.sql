-- 011_create_failed_jobs.sql
-- Stores dead-letter queue entries so they can be inspected and retried.
--
-- NOTE: id is VARCHAR(36) rather than a native UUID type so that the GORM
-- model's gorm:"type:varchar(36)" tag produces matching DDL for both
-- PostgreSQL and SQLite (used in unit tests). In a PostgreSQL-only deployment
-- the column can be changed to UUID without data loss.

CREATE TABLE IF NOT EXISTS failed_jobs (
    id           VARCHAR(36)  PRIMARY KEY,
    job_id       VARCHAR(255) NOT NULL,
    queue_name   VARCHAR(100) NOT NULL,
    payload      TEXT         NOT NULL,
    error_message TEXT,
    retry_count  INT          NOT NULL DEFAULT 0,
    max_retries  INT          NOT NULL DEFAULT 3,
    status       VARCHAR(50)  NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'recovering', 'exhausted')),
    last_retry_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_failed_jobs_job_id     ON failed_jobs (job_id);
CREATE INDEX IF NOT EXISTS idx_failed_jobs_queue_name ON failed_jobs (queue_name);
CREATE INDEX IF NOT EXISTS idx_failed_jobs_status     ON failed_jobs (status);
