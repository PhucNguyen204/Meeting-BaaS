package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// BotV2Row is the V2 view of the bots table. Extends the legacy BotRow with
// the columns added in migrations/008_extend_bots_v2.up.sql so the api-server
// can speak the Meeting BaaS v2 contract.
type BotV2Row struct {
	ID                  string
	BotUUID             string
	TenantID            string
	APIKeyID            string
	UserID              int64
	MeetingURL          string
	MeetingProv         string
	BotName             string
	BotImageURL         string
	EntryMessage        string
	RecordingMode       string
	AllowMultipleBots   bool
	Extra               []byte // raw JSONB
	IdempotencyKey      string
	DeduplicationHash   string

	TranscriptionEnabled        bool
	TranscriptionProvider       string
	TranscriptionBYOKSecretID   string

	StreamingEnabled         bool
	StreamingInputURL        string
	StreamingOutputURL       string
	StreamingAudioFrequency  int32

	JoinAt           *time.Time
	CallbackURL      string
	CallbackSecretID string

	Status       string
	EndReason    string
	ErrorCode    string
	ErrorMessage string

	WebhookURL string
	Environ    string

	CreatedAt          time.Time
	StartedAt          *time.Time
	JoinedAt           *time.Time
	RecordingStartedAt *time.Time
	RecordingEndedAt   *time.Time
	EndedAt            *time.Time
	DataDeletedAt      *time.Time
}

// InsertV2 creates a v2 bot row. Returns the generated id.
//
// row.IdempotencyKey + tenant_id form a partial UNIQUE; a duplicate insert
// surfaces via pgx's "unique_violation" SQLSTATE 23505 which the api-server
// idempotency layer should already have prevented.
func (r *BotRepo) InsertV2(ctx context.Context, row BotV2Row) (string, error) {
	if r == nil || r.pool == nil {
		return "", fmt.Errorf("bot_repo: nil pool")
	}

	const q = `
        INSERT INTO bots (
            bot_uuid, tenant_id, api_key_id, user_id,
            meeting_url, meeting_provider, bot_name, enter_message, bot_image_url,
            recording_mode, allow_multiple_bots, extra,
            idempotency_key, deduplication_hash,
            transcription_enabled, transcription_provider, transcription_byok_secret_id,
            streaming_enabled, streaming_input_url, streaming_output_url, streaming_audio_frequency,
            join_at, callback_url, callback_secret_id,
            status, webhook_url, environ
        ) VALUES (
            COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()),
            NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, $4,
            $5, $6, $7, $8, NULLIF($9, ''),
            $10, $11, COALESCE(NULLIF($12, '')::jsonb, '{}'::jsonb),
            NULLIF($13, ''), NULLIF($14, ''),
            $15, NULLIF($16, ''), NULLIF($17, '')::uuid,
            $18, NULLIF($19, ''), NULLIF($20, ''), NULLIF($21, 0)::int,
            $22, NULLIF($23, ''), NULLIF($24, '')::uuid,
            $25, NULLIF($26, ''), $27
        ) RETURNING id::text`

	var id string
	err := r.pool.QueryRow(ctx, q,
		row.BotUUID, row.TenantID, row.APIKeyID, row.UserID,
		row.MeetingURL, row.MeetingProv, row.BotName, row.EntryMessage, row.BotImageURL,
		defaultStr(row.RecordingMode, "speaker_view"), row.AllowMultipleBots, string(row.Extra),
		row.IdempotencyKey, row.DeduplicationHash,
		row.TranscriptionEnabled, row.TranscriptionProvider, row.TranscriptionBYOKSecretID,
		row.StreamingEnabled, row.StreamingInputURL, row.StreamingOutputURL, row.StreamingAudioFrequency,
		row.JoinAt, row.CallbackURL, row.CallbackSecretID,
		defaultStr(row.Status, "queued"), row.WebhookURL, defaultStr(row.Environ, "prod"),
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("bot_repo: insert v2: %w", err)
	}
	return id, nil
}

// GetV2 fetches a single row in the v2 shape.
func (r *BotRepo) GetV2(ctx context.Context, id string) (BotV2Row, error) {
	const q = `
        SELECT id::text, bot_uuid::text,
               COALESCE(tenant_id::text, ''), COALESCE(api_key_id::text, ''),
               user_id,
               meeting_url, meeting_provider, bot_name,
               COALESCE(bot_image_url, ''),
               COALESCE(enter_message, ''),
               recording_mode, allow_multiple_bots, extra::text,
               COALESCE(idempotency_key, ''), COALESCE(deduplication_hash, ''),
               transcription_enabled, COALESCE(transcription_provider, ''),
               COALESCE(transcription_byok_secret_id::text, ''),
               streaming_enabled, COALESCE(streaming_input_url, ''),
               COALESCE(streaming_output_url, ''), COALESCE(streaming_audio_frequency, 0),
               join_at, COALESCE(callback_url, ''),
               COALESCE(callback_secret_id::text, ''),
               status, COALESCE(end_reason, ''),
               COALESCE(error_code, ''), COALESCE(error_message, ''),
               COALESCE(webhook_url, ''), environ,
               created_at, started_at, joined_at,
               recording_started, recording_ended, ended_at, data_deleted_at
        FROM bots
        WHERE id = $1::uuid AND deleted_at IS NULL`

	var row BotV2Row
	var extraStr string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&row.ID, &row.BotUUID,
		&row.TenantID, &row.APIKeyID, &row.UserID,
		&row.MeetingURL, &row.MeetingProv, &row.BotName,
		&row.BotImageURL, &row.EntryMessage,
		&row.RecordingMode, &row.AllowMultipleBots, &extraStr,
		&row.IdempotencyKey, &row.DeduplicationHash,
		&row.TranscriptionEnabled, &row.TranscriptionProvider, &row.TranscriptionBYOKSecretID,
		&row.StreamingEnabled, &row.StreamingInputURL, &row.StreamingOutputURL, &row.StreamingAudioFrequency,
		&row.JoinAt, &row.CallbackURL, &row.CallbackSecretID,
		&row.Status, &row.EndReason, &row.ErrorCode, &row.ErrorMessage,
		&row.WebhookURL, &row.Environ,
		&row.CreatedAt, &row.StartedAt, &row.JoinedAt,
		&row.RecordingStartedAt, &row.RecordingEndedAt, &row.EndedAt, &row.DataDeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BotV2Row{}, ErrNotFound
	}
	if err != nil {
		return BotV2Row{}, fmt.Errorf("bot_repo: get v2: %w", err)
	}
	row.Extra = []byte(extraStr)
	return row, nil
}

// GetStatusLight is a lightweight projection for GET /v2/bots/{id}/status.
// Skips the heavy fields (extra jsonb, raw payloads) and only fetches what
// the lightweight status endpoint actually serialises.
func (r *BotRepo) GetStatusLight(ctx context.Context, id string) (botStatusLight, error) {
	const q = `
        SELECT id::text, bot_uuid::text, status,
               COALESCE(end_reason, ''),
               created_at, started_at, joined_at,
               recording_started, recording_ended, ended_at
        FROM bots
        WHERE id = $1::uuid AND deleted_at IS NULL`
	var row botStatusLight
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&row.ID, &row.BotUUID, &row.Status, &row.EndReason,
		&row.CreatedAt, &row.StartedAt, &row.JoinedAt,
		&row.RecordingStartedAt, &row.RecordingEndedAt, &row.EndedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return botStatusLight{}, ErrNotFound
	}
	if err != nil {
		return botStatusLight{}, fmt.Errorf("bot_repo: get status light: %w", err)
	}
	return row, nil
}

// botStatusLight is unexported so handlers serialize it directly without us
// having to expose the field tags publicly.
type botStatusLight struct {
	ID                 string     `json:"id"`
	BotUUID            string     `json:"bot_uuid"`
	Status             string     `json:"status"`
	EndReason          string     `json:"end_reason,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	JoinedAt           *time.Time `json:"joined_at,omitempty"`
	RecordingStartedAt *time.Time `json:"recording_started,omitempty"`
	RecordingEndedAt   *time.Time `json:"recording_ended,omitempty"`
	EndedAt            *time.Time `json:"ended_at,omitempty"`
}

// MarkDataDeleted flags a bot row as having its artifacts purged. Used by the
// data_retention_jobs worker to record completion without removing the row.
func (r *BotRepo) MarkDataDeleted(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE bots SET data_deleted_at = NOW() WHERE id = $1::uuid AND data_deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("bot_repo: mark data deleted: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
