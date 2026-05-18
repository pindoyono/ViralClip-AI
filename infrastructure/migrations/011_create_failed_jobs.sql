-- 011_create_failed_jobs.sql
-- Stores dead-letter queue entries so they can be inspected and retried.

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
