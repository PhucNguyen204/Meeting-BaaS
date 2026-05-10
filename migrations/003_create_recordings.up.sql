-- 003_create_recordings: tracks output mp4/wav segments per bot.
-- Decoupled from bots so we can have multiple segments (pause/resume).

CREATE TABLE IF NOT EXISTS recordings (
    id              BIGSERIAL PRIMARY KEY,
    bot_id          UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    segment_index   SMALLINT NOT NULL DEFAULT 0,
    local_path      TEXT,
    s3_bucket       VARCHAR(255),
    s3_key          TEXT,
    s3_url          TEXT,
    file_size_bytes BIGINT,
    duration_ms     INTEGER,
    format          VARCHAR(10) NOT NULL DEFAULT 'mp4', -- 'mp4'|'wav'
    upload_status   VARCHAR(20) NOT NULL DEFAULT 'pending', -- 'pending'|'uploading'|'completed'|'failed'
    upload_error    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    uploaded_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recordings_bot      ON recordings(bot_id);
CREATE INDEX IF NOT EXISTS idx_recordings_status   ON recordings(upload_status);
