package postgres

import (
	"context"
	"fmt"
	"time"
)

// --- alerts ---------------------------------------------------------------

// AlertsRepo handles CRUD over alerts (migrations/009).
type AlertsRepo struct {
	pool *Pool
}

// NewAlertsRepo constructs an AlertsRepo.
func NewAlertsRepo(pool *Pool) *AlertsRepo { return &AlertsRepo{pool: pool} }

// AlertRow mirrors the alerts table.
type AlertRow struct {
	ID         string     `json:"id"`
	Code       string     `json:"code"`
	Severity   string     `json:"severity"`
	Message    string     `json:"message"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// ListOpen returns alerts that have not been resolved yet, newest first.
func (r *AlertsRepo) ListOpen(ctx context.Context, tenantID string, limit int) ([]AlertRow, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("alerts_repo: nil pool")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `
        SELECT id::text, code, severity, message, created_at, resolved_at
        FROM alerts
        WHERE tenant_id = $1::uuid AND resolved_at IS NULL
        ORDER BY created_at DESC
        LIMIT $2`
	rows, err := r.pool.Query(ctx, q, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("alerts_repo: list: %w", err)
	}
	defer rows.Close()
	var out []AlertRow
	for rows.Next() {
		var a AlertRow
		if err := rows.Scan(&a.ID, &a.Code, &a.Severity, &a.Message, &a.CreatedAt, &a.ResolvedAt); err != nil {
			return nil, fmt.Errorf("alerts_repo: scan: %w", err)
		}
		out = append(out, a)
	}
	return out, nil
}

// --- usage_records --------------------------------------------------------

// UsageRepo handles CRUD over usage_records (migrations/009).
type UsageRepo struct {
	pool *Pool
}

// NewUsageRepo constructs a UsageRepo.
func NewUsageRepo(pool *Pool) *UsageRepo { return &UsageRepo{pool: pool} }

// UsageSummary is the aggregate returned by GET /v2/usage.
type UsageSummary struct {
	TenantID       string  `json:"tenant_id"`
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
	TokensUsed     float64 `json:"tokens_used"`
	MinutesUsed    float64 `json:"minutes_used"`
	TotalCostCents float64 `json:"total_cost_cents"`
}

// Summarize aggregates usage_records over the given window. start / end are
// inclusive / exclusive respectively.
func (r *UsageRepo) Summarize(ctx context.Context, tenantID string, start, end time.Time) (UsageSummary, error) {
	if r == nil || r.pool == nil {
		return UsageSummary{}, fmt.Errorf("usage_repo: nil pool")
	}
	const q = `
        SELECT
            COALESCE(SUM(amount) FILTER (WHERE kind = 'token'),   0)::float8,
            COALESCE(SUM(amount) FILTER (WHERE kind IN ('minute','transcription_min')), 0)::float8,
            COALESCE(SUM(total_cents), 0)::float8
        FROM usage_records
        WHERE tenant_id = $1::uuid AND recorded_at >= $2 AND recorded_at < $3`
	var s UsageSummary
	s.TenantID = tenantID
	s.PeriodStart = start.UTC().Format(time.RFC3339)
	s.PeriodEnd = end.UTC().Format(time.RFC3339)
	if err := r.pool.QueryRow(ctx, q, tenantID, start, end).Scan(
		&s.TokensUsed, &s.MinutesUsed, &s.TotalCostCents,
	); err != nil {
		return UsageSummary{}, fmt.Errorf("usage_repo: summarize: %w", err)
	}
	return s, nil
}

// --- webhook_event_outbox -------------------------------------------------

// OutboxRepo writes events into webhook_event_outbox (migrations/010).
type OutboxRepo struct {
	pool *Pool
}

// NewOutboxRepo constructs an OutboxRepo.
func NewOutboxRepo(pool *Pool) *OutboxRepo { return &OutboxRepo{pool: pool} }

// Append records one outbox event. Caller is expected to be inside the same
// transaction as the row mutation that triggered the event; the simple
// signature here is enough for the MVP api-server path.
func (r *OutboxRepo) Append(ctx context.Context, tenantID, botID, eventCode string, payload []byte) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("outbox_repo: nil pool")
	}
	const q = `
        INSERT INTO webhook_event_outbox (tenant_id, bot_id, event_code, payload)
        VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, COALESCE(NULLIF($4, '')::jsonb, '{}'::jsonb))`
	if _, err := r.pool.Exec(ctx, q, tenantID, botID, eventCode, string(payload)); err != nil {
		return fmt.Errorf("outbox_repo: append: %w", err)
	}
	return nil
}

// --- data_retention_jobs --------------------------------------------------

// RetentionRepo schedules artifact-deletion jobs.
type RetentionRepo struct {
	pool *Pool
}

// NewRetentionRepo constructs a RetentionRepo.
func NewRetentionRepo(pool *Pool) *RetentionRepo { return &RetentionRepo{pool: pool} }

// Schedule inserts a pending retention job for the given bot. scheduledAt
// may be zero, in which case NOW() is used (= delete-data API path).
func (r *RetentionRepo) Schedule(ctx context.Context, tenantID, botID string, scheduledAt time.Time) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("retention_repo: nil pool")
	}
	if scheduledAt.IsZero() {
		scheduledAt = time.Now()
	}
	const q = `
        INSERT INTO data_retention_jobs (tenant_id, bot_id, scheduled_at)
        VALUES ($1::uuid, $2::uuid, $3)`
	if _, err := r.pool.Exec(ctx, q, tenantID, botID, scheduledAt); err != nil {
		return fmt.Errorf("retention_repo: schedule: %w", err)
	}
	return nil
}
