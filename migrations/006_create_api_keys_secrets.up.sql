-- 006_create_api_keys_secrets: hashed API keys + secrets vault for OAuth tokens / BYOK.
-- Secrets store ciphertext only; the data encryption key (DEK) lives in KMS / Vault.

CREATE TABLE IF NOT EXISTS secrets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    purpose         VARCHAR(40) NOT NULL
                    CHECK (purpose IN ('oauth_refresh','transcription_byok',
                                       'webhook_callback','streaming_auth','generic')),
    kms_key_id      VARCHAR(255) NOT NULL,
    ciphertext      BYTEA NOT NULL,
    nonce           BYTEA NOT NULL,
    rotated_from    UUID REFERENCES secrets(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rotated_at      TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_secrets_tenant
    ON secrets (tenant_id, purpose) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS api_keys (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    name                VARCHAR(120) NOT NULL,
    key_prefix          CHAR(8) NOT NULL,
    key_hash            BYTEA NOT NULL UNIQUE,
    scopes              TEXT[] NOT NULL DEFAULT '{bots:write,bots:read}',
    rate_limit_per_min  INTEGER NOT NULL DEFAULT 600,
    last_used_at        TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_active
    ON api_keys (tenant_id)
    WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at > NOW());
