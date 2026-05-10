-- 002_create_bot_events: audit log of state machine transitions and webhook events.
-- One row per emitted event (joining_call, in_waiting_room, in_call_recording,
-- recording_paused, recording_resumed, call_ended, bot_rejected, …).

CREATE TABLE IF NOT EXISTS bot_events (
    id              BIGSERIAL PRIMARY KEY,
    bot_id          UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    event_code      VARCHAR(50) NOT NULL,
    event_data      JSONB DEFAULT '{}'::jsonb,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered       BOOLEAN NOT NULL DEFAULT FALSE,
    delivery_error  TEXT,
    retry_count     SMALLINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_bot_events_bot   ON bot_events(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_events_code  ON bot_events(event_code);
CREATE INDEX IF NOT EXISTS idx_bot_events_sent  ON bot_events(sent_at DESC);
