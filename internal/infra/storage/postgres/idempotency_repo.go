package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// IdempotencyHit describes what BeginRequest found on a replay.
type IdempotencyHit struct {
	// IsFirst is true when the (tenant_id, key) row did not previously exist
	// and we successfully inserted a placeholder. The handler should run as
	// usual and call CompleteRequest afterwards.
	IsFirst bool

	// HashMatch is true when the previously-recorded request_hash matches
	// the current request body. If false and IsFirst is false, the caller
	// should reject with 409 IDEMPOTENCY_CONFLICT.
	HashMatch bool

	// Status and Body carry the cached response when IsFirst is false and
	// the previous request has already finished (response_status is non-NULL).
	// Empty (0, nil) when the previous request is still in flight.
	Status int
	Body   []byte
}

// IdempotencyRepo manages the per-tenant idempotency window.
type IdempotencyRepo struct {
	pool *Pool
}

// NewIdempotencyRepo constructs an IdempotencyRepo.
func NewIdempotencyRepo(pool *Pool) *IdempotencyRepo {
	return &IdempotencyRepo{pool: pool}
}

// BeginRequest reserves the (tenant_id, key) slot. On the first call it
// inserts a row with request_hash and returns IsFirst=true; on subsequent
// calls it returns the cached state so the handler can short-circuit.
//
// requestHash should be sha256(body) (or method+path+body) computed by the
// middleware. Pass nil to skip hash comparison (not recommended).
func (r *IdempotencyRepo) BeginRequest(ctx context.Context, tenantID, key string, requestHash []byte) (IdempotencyHit, error) {
	if r == nil || r.pool == nil {
		return IdempotencyHit{}, fmt.Errorf("idempotency_repo: nil pool")
	}

	const q = `
        INSERT INTO idempotency_keys (tenant_id, key, request_hash)
        VALUES ($1::uuid, $2, $3)
        ON CONFLICT (tenant_id, key) DO NOTHING
        RETURNING tenant_id::text`

	// Try to insert. If it returns a row, this is the first request.
	var inserted string
	err := r.pool.QueryRow(ctx, q, tenantID, key, requestHash).Scan(&inserted)
	if err == nil {
		return IdempotencyHit{IsFirst: true, HashMatch: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyHit{}, fmt.Errorf("idempotency_repo: insert: %w", err)
	}

	// Row already exists. Fetch the recorded hash + response.
	const sel = `
        SELECT request_hash,
               COALESCE(response_status, 0),
               COALESCE(response_body, 'null'::jsonb)::text
        FROM idempotency_keys
        WHERE tenant_id = $1::uuid AND key = $2`
	var storedHash []byte
	var status int
	var bodyStr string
	if err := r.pool.QueryRow(ctx, sel, tenantID, key).Scan(&storedHash, &status, &bodyStr); err != nil {
		return IdempotencyHit{}, fmt.Errorf("idempotency_repo: select: %w", err)
	}
	hit := IdempotencyHit{
		IsFirst:   false,
		HashMatch: bytes.Equal(storedHash, requestHash),
		Status:    status,
	}
	if bodyStr != "null" {
		hit.Body = []byte(bodyStr)
	}
	return hit, nil
}

// CompleteRequest records the response so future replays can short-circuit.
// Safe to call even after BeginRequest reported IsFirst=false (it will just
// overwrite, which is what we want for the rare retry-during-flight case).
func (r *IdempotencyRepo) CompleteRequest(ctx context.Context, tenantID, key string, status int, body []byte) error {
	const q = `
        UPDATE idempotency_keys
           SET response_status = $3,
               response_body   = COALESCE(NULLIF($4, '')::jsonb, 'null'::jsonb)
         WHERE tenant_id = $1::uuid AND key = $2`
	_, err := r.pool.Exec(ctx, q, tenantID, key, status, string(body))
	if err != nil {
		return fmt.Errorf("idempotency_repo: complete: %w", err)
	}
	return nil
}

// SweepExpired deletes rows past their expiry. Intended for a cron task.
func (r *IdempotencyRepo) SweepExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("idempotency_repo: sweep: %w", err)
	}
	return tag.RowsAffected(), nil
}
