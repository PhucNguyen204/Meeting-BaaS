-- 010_create_webhook_outbox: outbox pattern + endpoint registry for at-least-once
-- webhook delivery. The bot-worker / api-server INSERT into webhook_event_outbox
-- inside the same transaction that mutates `bots`; a separate delivery worker
-- (Phase 4) drains the outbox and writes to webhook_deliveries (created in 004).

CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    url                     TEXT NOT NULL,
    secret_id               UUID REFERENCES secrets(id) ON DELETE SET NULL,
    event_filter            TEXT[] NOT NULL DEFAULT '{*}',
    is_active               BOOLEAN NOT NULL DEFAULT TRUE,
    consecutive_failures    INTEGER NOT NULL DEFAULT 0,
    disabled_at             TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ,
    UNIQUE (tenant_id, url)
);
CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_active
    ON webhook_endpoints (tenant_id) WHERE is_active AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS webhook_event_outbox (
    id           BIGSERIAL PRIMARY KEY,
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    bot_id       UUID REFERENCES bots(id) ON DELETE SET NULL,
    event_code   VARCHAR(60) NOT NULL,
    payload      JSONB NOT NULL,
    processed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_outbox_unprocessed
    ON webhook_event_outbox (created_at) WHERE processed_at IS NULL;

CREATE TABLE IF NOT EXISTS data_retention_jobs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    bot_id       UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    scheduled_at TIMESTAMPTZ NOT NULL,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    status       VARCHAR(20) NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','running','done','failed')),
    error        TEXT
);
CREATE INDEX IF NOT EXISTS idx_retention_due
    ON data_retention_jobs (scheduled_at) WHERE status = 'pending';
