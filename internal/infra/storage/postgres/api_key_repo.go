package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// APIKeyPrefix is prepended to every plaintext token we issue.
//
// "mb_" (meeting bot) + 32 hex characters of randomness; first 8 of those
// (after the prefix) are stored separately as key_prefix so the dashboard
// can show a partial token without ever seeing the full secret.
const APIKeyPrefix = "mb_"

// APIKeyRow mirrors the api_keys table (migrations/006).
type APIKeyRow struct {
	ID              string
	TenantID        string
	CreatedBy       string
	Name            string
	KeyPrefix       string
	Scopes          []string
	RateLimitPerMin int32
	LastUsedAt      *time.Time
	ExpiresAt       *time.Time
	RevokedAt       *time.Time
	CreatedAt       time.Time
}

// APIKeyRepo handles CRUD over api_keys.
type APIKeyRepo struct {
	pool *Pool
}

// NewAPIKeyRepo constructs an APIKeyRepo.
func NewAPIKeyRepo(pool *Pool) *APIKeyRepo {
	return &APIKeyRepo{pool: pool}
}

// Issue mints a brand-new API key, stores its sha256 hash, and returns
// both the stored row and the *plaintext* token (only time it is ever
// returned). Caller is responsible for surfacing the plaintext to the
// user exactly once.
func (r *APIKeyRepo) Issue(ctx context.Context, tenantID, createdBy, name string, scopes []string, rateLimitPerMin int32) (*APIKeyRow, string, error) {
	if r == nil || r.pool == nil {
		return nil, "", fmt.Errorf("api_key_repo: nil pool")
	}
	if name == "" {
		return nil, "", fmt.Errorf("api_key_repo: name required")
	}
	if rateLimitPerMin <= 0 {
		rateLimitPerMin = 600
	}
	if len(scopes) == 0 {
		scopes = []string{"bots:write", "bots:read"}
	}

	// 16 random bytes -> 32 hex characters.
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", fmt.Errorf("api_key_repo: rand: %w", err)
	}
	plaintext := APIKeyPrefix + hex.EncodeToString(raw)

	hash := sha256.Sum256([]byte(plaintext))
	prefix := plaintext[len(APIKeyPrefix) : len(APIKeyPrefix)+8]

	const q = `
        INSERT INTO api_keys (tenant_id, created_by, name, key_prefix, key_hash,
                              scopes, rate_limit_per_min)
        VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7)
        RETURNING id::text, created_at`

	var row APIKeyRow
	row.TenantID = tenantID
	row.CreatedBy = createdBy
	row.Name = name
	row.KeyPrefix = prefix
	row.Scopes = scopes
	row.RateLimitPerMin = rateLimitPerMin
	err := r.pool.QueryRow(ctx, q,
		tenantID, createdBy, name, prefix, hash[:], scopes, rateLimitPerMin,
	).Scan(&row.ID, &row.CreatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("api_key_repo: insert: %w", err)
	}
	return &row, plaintext, nil
}

// LookupByPlaintext resolves a token presented by a client into the row that
// minted it. Returns ErrNotFound for unknown/revoked/expired tokens so the
// caller can convert to a single 401 response without leaking which case
// applies.
func (r *APIKeyRepo) LookupByPlaintext(ctx context.Context, plaintext string) (*APIKeyRow, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("api_key_repo: nil pool")
	}
	hash := sha256.Sum256([]byte(plaintext))

	const q = `
        SELECT id::text, tenant_id::text,
               COALESCE(created_by::text, ''), name, key_prefix, scopes,
               rate_limit_per_min, last_used_at, expires_at, revoked_at, created_at
        FROM api_keys
        WHERE key_hash = $1
          AND revoked_at IS NULL
          AND (expires_at IS NULL OR expires_at > NOW())`

	var row APIKeyRow
	err := r.pool.QueryRow(ctx, q, hash[:]).Scan(
		&row.ID, &row.TenantID, &row.CreatedBy, &row.Name, &row.KeyPrefix,
		&row.Scopes, &row.RateLimitPerMin,
		&row.LastUsedAt, &row.ExpiresAt, &row.RevokedAt, &row.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("api_key_repo: lookup: %w", err)
	}
	return &row, nil
}

// MarkUsed bumps last_used_at = NOW(). Best-effort; failure is logged but
// not propagated since it would defeat the point of the auth middleware
// staying fast.
func (r *APIKeyRepo) MarkUsed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1::uuid`, id)
	if err != nil {
		return fmt.Errorf("api_key_repo: mark used: %w", err)
	}
	return nil
}

// Revoke sets revoked_at = NOW().
func (r *APIKeyRepo) Revoke(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = NOW() WHERE id = $1::uuid AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("api_key_repo: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
