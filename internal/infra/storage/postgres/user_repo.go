package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// UserRow mirrors the users table (migrations/005).
type UserRow struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	FullName     string
	Role         string
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserRepo handles CRUD over users.
type UserRepo struct {
	pool *Pool
}

// NewUserRepo constructs a UserRepo bound to pool.
func NewUserRepo(pool *Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

// Insert creates a new user and returns its id.
func (r *UserRepo) Insert(ctx context.Context, tenantID, email, passwordHash, fullName, role string) (string, error) {
	if r == nil || r.pool == nil {
		return "", fmt.Errorf("user_repo: nil pool")
	}
	if role == "" {
		role = "member"
	}
	const q = `
        INSERT INTO users (tenant_id, email, password_hash, full_name, role)
        VALUES ($1::uuid, $2, NULLIF($3, ''), $4, $5)
        RETURNING id::text`

	var id string
	err := r.pool.QueryRow(ctx, q, tenantID, email, passwordHash, fullName, role).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("user_repo: insert: %w", err)
	}
	return id, nil
}

// GetByEmail fetches a user by tenant + email. ErrNotFound on miss.
func (r *UserRepo) GetByEmail(ctx context.Context, tenantID, email string) (UserRow, error) {
	const q = `
        SELECT id::text, tenant_id::text, email,
               COALESCE(password_hash, ''), COALESCE(full_name, ''), role,
               last_login_at, created_at, updated_at
        FROM users
        WHERE tenant_id = $1::uuid AND email = $2 AND deleted_at IS NULL`
	var row UserRow
	err := r.pool.QueryRow(ctx, q, tenantID, email).Scan(
		&row.ID, &row.TenantID, &row.Email,
		&row.PasswordHash, &row.FullName, &row.Role,
		&row.LastLoginAt, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRow{}, ErrNotFound
	}
	if err != nil {
		return UserRow{}, fmt.Errorf("user_repo: get by email: %w", err)
	}
	return row, nil
}

// TouchLastLogin updates last_login_at = NOW(). Called by auth middleware.
func (r *UserRepo) TouchLastLogin(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1::uuid`, id)
	if err != nil {
		return fmt.Errorf("user_repo: touch last login: %w", err)
	}
	return nil
}
