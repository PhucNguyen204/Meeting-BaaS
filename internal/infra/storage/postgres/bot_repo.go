package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned by Get when the row does not exist.
var ErrNotFound = errors.New("postgres: not found")

// BotRow is the canonical Go view of the `bots` table.
//
// Mirrors migrations/001_create_bots.up.sql.
type BotRow struct {
	ID             string     `db:"id"`
	BotUUID        string     `db:"bot_uuid"`
	SessionID      string     `db:"session_id"`
	UserID         int64      `db:"user_id"`
	MeetingURL     string     `db:"meeting_url"`
	MeetingProv    string     `db:"meeting_provider"`
	MeetingID      string     `db:"meeting_id"`
	BotName        string     `db:"bot_name"`
	EnterMessage   string     `db:"enter_message"`
	RecordingMode  string     `db:"recording_mode"`
	Status         string     `db:"status"`
	EndReason      string     `db:"end_reason"`
	ErrorMessage   string     `db:"error_message"`
	CreatedAt      time.Time  `db:"created_at"`
	StartedAt      *time.Time `db:"started_at"`
	JoinedAt       *time.Time `db:"joined_at"`
	RecordingStart *time.Time `db:"recording_started"`
	RecordingEnd   *time.Time `db:"recording_ended"`
	EndedAt        *time.Time `db:"ended_at"`
	WebhookURL     string     `db:"webhook_url"`
	Environ        string     `db:"environ"`
	RetryCount     int16      `db:"retry_count"`
	ShouldRetry    bool       `db:"should_retry"`
}

// BotRepo is the data-access object for `bots`.
type BotRepo struct {
	pool *Pool
}

// NewBotRepo constructs a repo bound to pool.
func NewBotRepo(pool *Pool) *BotRepo {
	return &BotRepo{pool: pool}
}

// Insert creates a new bots row and returns the generated id.
func (r *BotRepo) Insert(ctx context.Context, row BotRow) (string, error) {
	if r == nil || r.pool == nil {
		return "", fmt.Errorf("bot_repo: nil pool")
	}
	const q = `
        INSERT INTO bots (
            bot_uuid, session_id, user_id, meeting_url, meeting_provider, meeting_id,
            bot_name, enter_message, recording_mode, status,
            webhook_url, environ
        ) VALUES (
            $1, $2, $3, $4, $5, $6,
            $7, $8, $9, $10,
            $11, $12
        ) RETURNING id::text`

	var id string
	err := r.pool.QueryRow(ctx, q,
		row.BotUUID, row.SessionID, row.UserID, row.MeetingURL, row.MeetingProv, row.MeetingID,
		row.BotName, row.EnterMessage, row.RecordingMode, defaultStr(row.Status, "created"),
		row.WebhookURL, defaultStr(row.Environ, "prod"),
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("bot_repo: insert: %w", err)
	}
	return id, nil
}

// Get fetches a single row by primary id.
func (r *BotRepo) Get(ctx context.Context, id string) (BotRow, error) {
	const q = `
        SELECT
            id::text, bot_uuid::text,
            COALESCE(session_id, ''),
            user_id,
            meeting_url, meeting_provider,
            COALESCE(meeting_id, ''),
            bot_name,
            COALESCE(enter_message, ''),
            recording_mode, status,
            COALESCE(end_reason, ''),
            COALESCE(error_message, ''),
            created_at, started_at, joined_at,
            recording_started, recording_ended, ended_at,
            COALESCE(webhook_url, ''),
            environ, retry_count, should_retry
        FROM bots WHERE id = $1::uuid`

	var row BotRow
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&row.ID, &row.BotUUID, &row.SessionID, &row.UserID,
		&row.MeetingURL, &row.MeetingProv, &row.MeetingID,
		&row.BotName, &row.EnterMessage, &row.RecordingMode, &row.Status,
		&row.EndReason, &row.ErrorMessage,
		&row.CreatedAt, &row.StartedAt, &row.JoinedAt,
		&row.RecordingStart, &row.RecordingEnd, &row.EndedAt,
		&row.WebhookURL, &row.Environ, &row.RetryCount, &row.ShouldRetry,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BotRow{}, ErrNotFound
	}
	if err != nil {
		return BotRow{}, fmt.Errorf("bot_repo: get: %w", err)
	}
	return row, nil
}

// UpdateStatus atomically advances the bot's lifecycle state and stamps the
// matching timestamp column. Pass empty endReason / errorMessage to leave them.
func (r *BotRepo) UpdateStatus(ctx context.Context, id, status, endReason, errorMessage string) error {
	const q = `
        UPDATE bots
           SET status        = $2,
               end_reason    = COALESCE(NULLIF($3, ''), end_reason),
               error_message = COALESCE(NULLIF($4, ''), error_message),
               started_at    = COALESCE(started_at, CASE WHEN $2 = 'initializing' THEN NOW() END),
               joined_at     = COALESCE(joined_at,  CASE WHEN $2 = 'in_call'      THEN NOW() END),
               recording_started = COALESCE(recording_started, CASE WHEN $2 = 'recording' THEN NOW() END),
               recording_ended   = COALESCE(recording_ended,   CASE WHEN $2 IN ('cleanup','completed','failed') THEN NOW() END),
               ended_at      = COALESCE(ended_at, CASE WHEN $2 IN ('completed','failed','terminated') THEN NOW() END)
         WHERE id = $1::uuid`
	tag, err := r.pool.Exec(ctx, q, id, status, endReason, errorMessage)
	if err != nil {
		return fmt.Errorf("bot_repo: update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
