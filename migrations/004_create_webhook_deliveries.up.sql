-- 004_create_webhook_deliveries: persistent record of every webhook attempt.
-- Used by the webhook sender to retry failed deliveries on restart.

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              BIGSERIAL PRIMARY KEY,
    bot_id          UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    event_id        BIGINT REFERENCES bot_events(id) ON DELETE SET NULL,
    webhook_url     TEXT NOT NULL,
    payload         JSONB NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending', -- 'pending'|'in_flight'|'succeeded'|'failed'|'circuit_open'
    http_status     SMALLINT,
    last_error      TEXT,
    attempt         SMALLINT NOT NULL DEFAULT 0,
    next_retry_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_bot      ON webhook_deliveries(bot_id);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status   ON webhook_deliveries(status);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_retry    ON webhook_deliveries(next_retry_at)
    WHERE status IN ('pending', 'failed');
