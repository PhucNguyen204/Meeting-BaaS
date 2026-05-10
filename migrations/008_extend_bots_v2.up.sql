-- 008_extend_bots_v2: bring the bots table up to the Meeting BaaS v2 contract.
-- Additive: every column is nullable / has a default so older rows survive untouched.

ALTER TABLE bots
    ADD COLUMN IF NOT EXISTS tenant_id                    UUID
        REFERENCES tenants(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS api_key_id                   UUID
        REFERENCES api_keys(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS idempotency_key              VARCHAR(255),
    ADD COLUMN IF NOT EXISTS deduplication_hash           CHAR(64),
    ADD COLUMN IF NOT EXISTS bot_image_url                TEXT,
    ADD COLUMN IF NOT EXISTS allow_multiple_bots          BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS extra                        JSONB   NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS transcription_enabled        BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS transcription_provider       VARCHAR(40),
    ADD COLUMN IF NOT EXISTS transcription_byok_secret_id UUID
        REFERENCES secrets(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS streaming_enabled            BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS streaming_input_url          TEXT,
    ADD COLUMN IF NOT EXISTS streaming_output_url         TEXT,
    ADD COLUMN IF NOT EXISTS streaming_audio_frequency    INTEGER,
    ADD COLUMN IF NOT EXISTS join_at                      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS callback_url                 TEXT,
    ADD COLUMN IF NOT EXISTS callback_secret_id           UUID
        REFERENCES secrets(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS data_deleted_at              TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_at                   TIMESTAMPTZ;

-- The session_id column predates v2; v1/v2 both use bot_uuid as the public id.
-- We keep session_id for forward compatibility with logs.

-- Unique idempotency window per tenant. Filtered to non-null so legacy rows
-- without an idempotency key don't fight for the partial index.
CREATE UNIQUE INDEX IF NOT EXISTS uq_bots_tenant_idempotency
    ON bots (tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Dedup window (allow_multiple_bots = false) — backstop for the Redis fast path.
CREATE UNIQUE INDEX IF NOT EXISTS uq_bots_tenant_dedup
    ON bots (tenant_id, deduplication_hash)
    WHERE deduplication_hash IS NOT NULL AND allow_multiple_bots = FALSE;

-- Index for the scheduler that wakes up "scheduled" bots when join_at arrives.
CREATE INDEX IF NOT EXISTS idx_bots_join_at
    ON bots (join_at) WHERE status = 'scheduled';

-- Tenant + created_at for dashboard list.
CREATE INDEX IF NOT EXISTS idx_bots_tenant_created
    ON bots (tenant_id, created_at DESC) WHERE deleted_at IS NULL;
