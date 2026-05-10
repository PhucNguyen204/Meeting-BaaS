-- 001_create_bots: source-of-truth row per bot session.
-- Replaces deployments/docker/postgres-init.sql `bot_sessions` table.
-- Versioned migrations preferred for Phase 3 onwards.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS bots (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bot_uuid            UUID NOT NULL,
    session_id          VARCHAR(255),
    user_id             BIGINT NOT NULL,
    user_token          VARCHAR(512),

    -- Meeting identifiers
    meeting_url         TEXT NOT NULL,
    meeting_provider    VARCHAR(20) NOT NULL DEFAULT 'unknown', -- 'Meet'|'Teams'|'Zoom'
    meeting_id          VARCHAR(512),

    -- Bot configuration
    bot_name            VARCHAR(255) NOT NULL DEFAULT 'Meeting Bot',
    enter_message       TEXT,
    recording_mode      VARCHAR(20) NOT NULL DEFAULT 'speaker_view',

    -- Lifecycle
    status              VARCHAR(30) NOT NULL DEFAULT 'created',
    end_reason          VARCHAR(50),
    error_message       TEXT,

    -- Timing
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at          TIMESTAMPTZ,
    joined_at           TIMESTAMPTZ,
    recording_started   TIMESTAMPTZ,
    recording_ended     TIMESTAMPTZ,
    ended_at            TIMESTAMPTZ,

    -- Output
    recording_path      TEXT,
    s3_recording_url    TEXT,
    mp4_s3_path         TEXT,

    -- Timeouts
    waiting_room_timeout_s  INTEGER NOT NULL DEFAULT 300,
    noone_joined_timeout_s  INTEGER NOT NULL DEFAULT 300,
    silence_timeout_s       INTEGER NOT NULL DEFAULT 300,

    -- Retry
    retry_count         SMALLINT NOT NULL DEFAULT 0,
    should_retry        BOOLEAN  NOT NULL DEFAULT FALSE,

    -- Environment
    environ             VARCHAR(20) NOT NULL DEFAULT 'prod',

    -- Webhook
    webhook_url         TEXT,
    api_key             VARCHAR(512),

    -- Streaming
    streaming_input     TEXT,
    streaming_output    TEXT,

    -- Extra
    extra               JSONB DEFAULT '{}'::jsonb,
    event_uuid          UUID
);

CREATE INDEX IF NOT EXISTS idx_bots_bot_uuid    ON bots(bot_uuid);
CREATE INDEX IF NOT EXISTS idx_bots_user_id     ON bots(user_id);
CREATE INDEX IF NOT EXISTS idx_bots_status      ON bots(status);
CREATE INDEX IF NOT EXISTS idx_bots_created_at  ON bots(created_at DESC);
