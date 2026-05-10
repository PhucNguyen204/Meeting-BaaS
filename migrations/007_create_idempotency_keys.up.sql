-- 007_create_idempotency_keys: store per-tenant idempotency window so retry-safe
-- POST endpoints return the original response on replay.

CREATE TABLE IF NOT EXISTS idempotency_keys (
    tenant_id       UUID NOT NULL,
    key             VARCHAR(255) NOT NULL,
    request_hash    BYTEA NOT NULL,
    response_status SMALLINT,
    response_body   JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
    PRIMARY KEY (tenant_id, key)
);
CREATE INDEX IF NOT EXISTS idx_idem_expiry ON idempotency_keys (expires_at);
