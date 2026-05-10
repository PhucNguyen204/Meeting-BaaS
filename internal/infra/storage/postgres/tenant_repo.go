package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TenantRow mirrors the tenants table (migrations/005).
type TenantRow struct {
	ID                string
	Name              string
	Slug              string
	Plan              string
	RetentionDays     int16
	StripeCustomerID  string
	Settings          []byte // raw JSONB
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TenantRepo handles CRUD over tenants.
type TenantRepo struct {
	pool *Pool
}

// NewTenantRepo constructs a TenantRepo bound to pool.
func NewTenantRepo(pool *Pool) *TenantRepo {
	return &TenantRepo{pool: pool}
}

// Insert creates a new tenant and returns its id. settings may be nil.
func (r *TenantRepo) Insert(ctx context.Context, name, slug, plan string, retentionDays int16, settings []byte) (string, error) {
	if r == nil || r.pool == nil {
		return "", fmt.Errorf("tenant_repo: nil pool")
	}
	const q = `
        INSERT INTO tenants (name, slug, plan, retention_days, settings)
        VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5, '')::jsonb, '{}'::jsonb))
        RETURNING id::text`
	if plan == "" {
		plan = "free"
	}
	if retentionDays == 0 {
		retentionDays = 7
	}

	var id string
	err := r.pool.QueryRow(ctx, q, name, slug, plan, retentionDays, string(settings)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("tenant_repo: insert: %w", err)
	}
	return id, nil
}

// GetBySlug fetches a tenant by its public slug. ErrNotFound on miss.
func (r *TenantRepo) GetBySlug(ctx context.Context, slug string) (TenantRow, error) {
	const q = `
        SELECT id::text, name, slug, plan, retention_days,
               COALESCE(stripe_customer_id, ''),
               settings::text, created_at, updated_at
        FROM tenants
        WHERE slug = $1 AND deleted_at IS NULL`
	var row TenantRow
	var settingsStr string
	err := r.pool.QueryRow(ctx, q, slug).Scan(
		&row.ID, &row.Name, &row.Slug, &row.Plan, &row.RetentionDays,
		&row.StripeCustomerID, &settingsStr, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return TenantRow{}, ErrNotFound
	}
	if err != nil {
		return TenantRow{}, fmt.Errorf("tenant_repo: get by slug: %w", err)
	}
	row.Settings = []byte(settingsStr)
	return row, nil
}

// GetByID fetches a tenant by its UUID id.
func (r *TenantRepo) GetByID(ctx context.Context, id string) (TenantRow, error) {
	const q = `
        SELECT id::text, name, slug, plan, retention_days,
               COALESCE(stripe_customer_id, ''),
               settings::text, created_at, updated_at
        FROM tenants
        WHERE id = $1::uuid AND deleted_at IS NULL`
	var row TenantRow
	var settingsStr string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&row.ID, &row.Name, &row.Slug, &row.Plan, &row.RetentionDays,
		&row.StripeCustomerID, &settingsStr, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return TenantRow{}, ErrNotFound
	}
	if err != nil {
		return TenantRow{}, fmt.Errorf("tenant_repo: get by id: %w", err)
	}
	row.Settings = []byte(settingsStr)
	return row, nil
}
