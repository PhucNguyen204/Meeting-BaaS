-- Meeting BaaS - PostgreSQL Schema
-- Designed from TS types.ts, events.ts, singleton.ts and MeetingBaaS API docs.
-- Each table maps to a real-world entity in the meeting lifecycle.

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================
-- 1. bot_sessions: Core table — one row per bot worker execution.
-- Maps to: TS MeetingParams (types.ts) + GLOBAL singleton state.
-- ============================================================
CREATE TABLE bot_sessions (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bot_uuid          UUID NOT NULL,                           -- MeetingParams.bot_uuid
    session_id        VARCHAR(255),                            -- MeetingParams.session_id
    user_id           INTEGER NOT NULL,                        -- MeetingParams.user_id
    user_token        VARCHAR(512),                            -- MeetingParams.user_token (masked in logs)

    -- Meeting identifiers
    meeting_url       TEXT NOT NULL,                            -- MeetingParams.meeting_url
    meeting_provider  VARCHAR(20) NOT NULL DEFAULT 'unknown',   -- 'google_meet' | 'microsoft_teams'
    meeting_id        VARCHAR(512),                             -- Parsed meeting ID

    -- Bot configuration
    bot_name          VARCHAR(255) NOT NULL DEFAULT 'Meeting Bot',
    enter_message     TEXT,                                     -- MeetingParams.enter_message
    recording_mode    VARCHAR(20) NOT NULL DEFAULT 'speaker_view', -- 'speaker_view' | 'gallery_view' | 'audio_only'

    -- Lifecycle state (mirrors state machine)
    status            VARCHAR(30) NOT NULL DEFAULT 'created',   -- 'created','initializing','waiting_room','in_call','recording','paused','cleanup','completed','failed','terminated'
    end_reason        VARCHAR(50),                              -- MeetingEndReason enum from TS
    error_message     TEXT,                                     -- Human-readable error

    -- Timing
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at        TIMESTAMPTZ,                              -- When bot-worker process started
    joined_at         TIMESTAMPTZ,                              -- When bot entered the call
    recording_started TIMESTAMPTZ,                              -- When recording began
    recording_ended   TIMESTAMPTZ,                              -- When recording stopped
    ended_at          TIMESTAMPTZ,                              -- When session fully terminated

    -- Recording output
    recording_path    TEXT,                                     -- Local/EFS path to MP4
    s3_recording_url  TEXT,                                     -- S3 URL after upload
    mp4_s3_path       TEXT,                                     -- MeetingParams.mp4_s3_path

    -- Timeouts (from MeetingParams.automatic_leave)
    waiting_room_timeout_s  INTEGER NOT NULL DEFAULT 300,
    noone_joined_timeout_s  INTEGER NOT NULL DEFAULT 300,
    silence_timeout_s       INTEGER NOT NULL DEFAULT 300,

    -- Retry
    retry_count       SMALLINT NOT NULL DEFAULT 0,
    should_retry      BOOLEAN NOT NULL DEFAULT FALSE,

    -- Environment
    environ           VARCHAR(20) NOT NULL DEFAULT 'prod',      -- 'local' | 'prod' | 'preprod'

    -- Webhook config
    webhook_url       TEXT,                                     -- MeetingParams.bots_webhook_url
    api_key           VARCHAR(512),                             -- MeetingParams.bots_api_key (masked in logs)

    -- Streaming config
    streaming_input   TEXT,
    streaming_output  TEXT,

    -- Extra metadata (JSON blob for extensibility)
    extra             JSONB DEFAULT '{}'::jsonb,

    -- Event reference
    event_uuid        UUID                                      -- MeetingParams.event?.uuid
);

CREATE INDEX idx_bot_sessions_bot_uuid ON bot_sessions(bot_uuid);
CREATE INDEX idx_bot_sessions_user_id ON bot_sessions(user_id);
CREATE INDEX idx_bot_sessions_status ON bot_sessions(status);
CREATE INDEX idx_bot_sessions_created_at ON bot_sessions(created_at DESC);

-- ============================================================
-- 2. bot_events: Webhook events sent during the session lifecycle.
-- Maps to: TS Events class (events.ts) — all status_change webhooks.
-- ============================================================
CREATE TABLE bot_events (
    id              BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
    event_code      VARCHAR(50) NOT NULL,   -- 'joining_call','in_waiting_room','in_call_not_recording','in_call_recording','recording_paused','recording_resumed','call_ended','bot_rejected','bot_removed','bot_removed_too_early','waiting_room_timeout','invalid_meeting_url','meeting_error','recording_succeeded','recording_failed'
    event_data      JSONB DEFAULT '{}'::jsonb,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered       BOOLEAN NOT NULL DEFAULT FALSE,
    delivery_error  TEXT,
    retry_count     SMALLINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_bot_events_session ON bot_events(session_id);
CREATE INDEX idx_bot_events_code ON bot_events(event_code);

-- ============================================================
-- 3. participants: Tracks participants seen during a meeting.
-- Maps to: TS GLOBAL.participantNames[] and SpeakerData.
-- ============================================================
CREATE TABLE participants (
    id              BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_spoken_at  TIMESTAMPTZ,
    is_bot          BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_participants_session ON participants(session_id);
CREATE UNIQUE INDEX idx_participants_session_name ON participants(session_id, name);

-- ============================================================
-- 4. speaker_timeline: Per-second speaker activity for transcript alignment.
-- Maps to: TS SpeakerData (types.ts) and speaker-manager.ts.
-- ============================================================
CREATE TABLE speaker_timeline (
    id              BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
    participant_id  BIGINT REFERENCES participants(id),
    speaker_name    VARCHAR(255) NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ,
    duration_ms     INTEGER GENERATED ALWAYS AS (
        EXTRACT(EPOCH FROM (ended_at - started_at)) * 1000
    ) STORED
);

CREATE INDEX idx_speaker_timeline_session ON speaker_timeline(session_id);

-- ============================================================
-- 5. transcripts: Stores transcription results (if STT is enabled).
-- Maps to: TS uploadTranscripts.ts — the final transcript payload.
-- ============================================================
CREATE TABLE transcripts (
    id              BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
    speaker_name    VARCHAR(255),
    content         TEXT NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ,
    language        VARCHAR(10),         -- 'en', 'fr', etc.
    is_final        BOOLEAN NOT NULL DEFAULT TRUE,
    confidence      REAL,                -- 0.0 - 1.0
    provider        VARCHAR(30)          -- 'Default', 'Gladia', 'RunPod'
);

CREATE INDEX idx_transcripts_session ON transcripts(session_id);

-- ============================================================
-- 6. recordings: Tracks recording files and their S3 upload status.
-- Separated from bot_sessions to support multiple recordings per session
-- (e.g., pause/resume creates segments).
-- ============================================================
CREATE TABLE recordings (
    id              BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
    segment_index   SMALLINT NOT NULL DEFAULT 0,
    local_path      TEXT,
    s3_bucket       VARCHAR(255),
    s3_key          TEXT,
    s3_url          TEXT,
    file_size_bytes BIGINT,
    duration_ms     INTEGER,
    format          VARCHAR(10) NOT NULL DEFAULT 'mp4',
    upload_status   VARCHAR(20) NOT NULL DEFAULT 'pending',  -- 'pending','uploading','completed','failed'
    upload_error    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    uploaded_at     TIMESTAMPTZ
);

CREATE INDEX idx_recordings_session ON recordings(session_id);
