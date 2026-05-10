-- postgres-init.sql — Schema for the meeting bot database.
-- Runs automatically via docker-entrypoint-initdb.d.

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Bot sessions table: one row per bot lifecycle.
CREATE TABLE IF NOT EXISTS bot_sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bot_uuid        UUID NOT NULL UNIQUE,
    session_id      VARCHAR(255),
    user_id         BIGINT,
    meeting_url     TEXT NOT NULL,
    provider        VARCHAR(20) NOT NULL,
    bot_name        VARCHAR(255) NOT NULL DEFAULT 'Recording Bot',
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    end_reason      VARCHAR(100),
    error_message   TEXT,
    recording_mode  VARCHAR(50) DEFAULT 'speaker_view',
    webhook_url     TEXT,
    
    -- Timing
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,
    
    -- Recording metadata
    mp4_s3_path     TEXT,
    duration_ms     BIGINT,
    
    -- Config snapshot (JSONB for flexibility)
    config_json     JSONB
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_bot_sessions_status ON bot_sessions(status);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_user_id ON bot_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_created_at ON bot_sessions(created_at DESC);

-- Participants table: tracks who was in the meeting.
CREATE TABLE IF NOT EXISTS participants (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id      UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_active       BOOLEAN DEFAULT true,
    
    UNIQUE(session_id, name)
);

CREATE INDEX IF NOT EXISTS idx_participants_session ON participants(session_id);

-- Webhook deliveries: tracks webhook delivery attempts.
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id      UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
    event_type      VARCHAR(50) NOT NULL,
    url             TEXT NOT NULL,
    payload         JSONB NOT NULL,
    status_code     INT,
    response_body   TEXT,
    attempt         INT NOT NULL DEFAULT 1,
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_session ON webhook_deliveries(session_id);

-- Grant all privileges to meetbot user
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO meetbot;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO meetbot;
