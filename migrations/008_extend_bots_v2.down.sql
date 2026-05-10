DROP INDEX IF EXISTS idx_bots_tenant_created;
DROP INDEX IF EXISTS idx_bots_join_at;
DROP INDEX IF EXISTS uq_bots_tenant_dedup;
DROP INDEX IF EXISTS uq_bots_tenant_idempotency;

ALTER TABLE bots
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS data_deleted_at,
    DROP COLUMN IF EXISTS callback_secret_id,
    DROP COLUMN IF EXISTS callback_url,
    DROP COLUMN IF EXISTS join_at,
    DROP COLUMN IF EXISTS streaming_audio_frequency,
    DROP COLUMN IF EXISTS streaming_output_url,
    DROP COLUMN IF EXISTS streaming_input_url,
    DROP COLUMN IF EXISTS streaming_enabled,
    DROP COLUMN IF EXISTS transcription_byok_secret_id,
    DROP COLUMN IF EXISTS transcription_provider,
    DROP COLUMN IF EXISTS transcription_enabled,
    DROP COLUMN IF EXISTS extra,
    DROP COLUMN IF EXISTS allow_multiple_bots,
    DROP COLUMN IF EXISTS bot_image_url,
    DROP COLUMN IF EXISTS deduplication_hash,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS api_key_id,
    DROP COLUMN IF EXISTS tenant_id;
