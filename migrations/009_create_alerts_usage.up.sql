-- 009_create_alerts_usage: operational alerts + per-tenant usage records.
-- usage_records is intentionally a plain table at MVP; production should switch
-- to declarative partitioning via pg_partman (see docs/database-design.md).

CREATE TABLE IF NOT EXISTS alerts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code        VARCHAR(40) NOT NULL,
    severity    VARCHAR(10) NOT NULL DEFAULT 'warn'
                CHECK (severity IN ('info','warn','error','critical')),
    message     TEXT NOT NULL,
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alerts_open
    ON alerts (tenant_id, severity, created_at DESC) WHERE resolved_at IS NULL;

CREATE TABLE IF NOT EXISTS usage_records (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    bot_id          UUID REFERENCES bots(id) ON DELETE SET NULL,
    kind            VARCHAR(20) NOT NULL
                    CHECK (kind IN ('token','minute','gb_storage','transcription_min')),
    amount          NUMERIC(12,4) NOT NULL,
    unit_cost_cents NUMERIC(12,4),
    total_cents     NUMERIC(14,4),
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_usage_tenant_time ON usage_records (tenant_id, recorded_at);
