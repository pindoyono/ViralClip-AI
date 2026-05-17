-- ViralClip AI - PostgreSQL initialization
-- This script runs when the PostgreSQL container is first created.

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create application database (if not already created via POSTGRES_DB)
-- The database is created by Docker env vars; we just set it up here.
\connect viralclip;

-- Set timezone
SET timezone = 'UTC';

-- Grant privileges (the app user is created by POSTGRES_USER env var)
GRANT ALL PRIVILEGES ON DATABASE viralclip TO viralclip;
