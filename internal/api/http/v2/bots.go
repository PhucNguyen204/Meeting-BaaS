package v2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	goredis "github.com/redis/go-redis/v9"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/middleware"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/queue"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/postgres"
)

// CreateBotRequest is the JSON body accepted by POST /v2/bots and (with
// join_at promoted to required) POST /v2/bots/scheduled.
//
// Field names match the Meeting BaaS v2 public reference; see
// docs/meetingbaas/api-v2/getting-started/sending-a-bot.md.
type CreateBotRequest struct {
	MeetingURL    string `json:"meeting_url"`
	BotName       string `json:"bot_name"`
	BotImage      string `json:"bot_image,omitempty"`
	EntryMessage  string `json:"entry_message,omitempty"`
	RecordingMode string `json:"recording_mode,omitempty"`

	TranscriptionEnabled bool                    `json:"transcription_enabled,omitempty"`
	TranscriptionConfig  *TranscriptionConfigReq `json:"transcription_config,omitempty"`

	StreamingEnabled        bool   `json:"streaming_enabled,omitempty"`
	StreamingInputURL       string `json:"streaming_input,omitempty"`
	StreamingOutputURL      string `json:"streaming_output,omitempty"`
	StreamingAudioFrequency int32  `json:"streaming_audio_frequency,omitempty"`

	TimeoutConfig *TimeoutConfigReq `json:"timeout_config,omitempty"`

	AllowMultipleBots *bool                  `json:"allow_multiple_bots,omitempty"`
	Extra             map[string]any         `json:"extra,omitempty"`
	CallbackConfig    *CallbackConfigReq     `json:"callback_config,omitempty"`
	JoinAt            *time.Time             `json:"join_at,omitempty"`
	WebhookURL        string                 `json:"bots_webhook_url,omitempty"`

	// V1 compat (older clients use these names; we accept both).
	UserID int64 `json:"user_id,omitempty"`
}

// TranscriptionConfigReq mirrors the public transcription_config block.
type TranscriptionConfigReq struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key,omitempty"`
}

// TimeoutConfigReq matches Meeting BaaS v2 timeout_config (seconds).
type TimeoutConfigReq struct {
	WaitingRoomTimeout int `json:"waiting_room_timeout,omitempty"`
	NoOneJoinedTimeout int `json:"no_one_joined_timeout,omitempty"`
	SilenceTimeout     int `json:"silence_timeout,omitempty"`
}

// CallbackConfigReq is per-bot webhook target (in addition to or in place of
// the account-level Webhook endpoint).
type CallbackConfigReq struct {
	URL    string `json:"url"`
	Secret string `json:"secret,omitempty"`
}

// CreateBotResponse is what we return from POST /v2/bots(/scheduled).
type CreateBotResponse struct {
	BotID    string `json:"bot_id"`
	StreamID string `json:"stream_id,omitempty"`
	Status   string `json:"status"`
}

// HandleCreateBot serves POST /v2/bots (immediate). The bot is inserted into
// the bots table and a job is XADDed onto the Redis stream so a controller
// picks it up and spawns a bot-worker.
func HandleCreateBot(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleCreate(w, r, deps, false)
	}
}

// HandleScheduleBot serves POST /v2/bots/scheduled. Same as create but
// requires join_at; status starts as "scheduled" and the row is not enqueued
// onto the stream yet (a separate scheduler worker promotes it when join_at
// arrives — Phase 5).
func HandleScheduleBot(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleCreate(w, r, deps, true)
	}
}

func handleCreate(w http.ResponseWriter, r *http.Request, deps Deps, scheduled bool) {
	tenantID := middleware.TenantFromContext(r.Context())
	apiKeyID := middleware.APIKeyIDFromContext(r.Context())
	if tenantID == "" {
		WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "tenant context missing")
		return
	}

	var req CreateBotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidParameters, "invalid JSON body: "+err.Error())
		return
	}
	if err := validateCreate(&req, scheduled); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidParameters, err.Error())
		return
	}

	provider := detectProvider(req.MeetingURL)
	allowMultiple := true
	if req.AllowMultipleBots != nil {
		allowMultiple = *req.AllowMultipleBots
	}

	extraJSON, _ := json.Marshal(req.Extra)

	// Dedup short-circuit when allow_multiple_bots=false. The DB has a
	// partial UNIQUE index as a backstop; Redis is the fast path.
	dedupHash := ""
	if !allowMultiple {
		dedupHash = dedupHashFor(req.MeetingURL, req.JoinAt)
		if existingID, hit := checkDedup(r.Context(), deps.Redis, tenantID, dedupHash); hit {
			WriteJSON(w, http.StatusOK, Envelope{
				Success: true,
				Data: CreateBotResponse{
					BotID:  existingID,
					Status: "queued",
				},
			})
			return
		}
	}

	status := "queued"
	if scheduled {
		status = "scheduled"
	}

	row := postgres.BotV2Row{
		TenantID:                tenantID,
		APIKeyID:                apiKeyID,
		UserID:                  req.UserID,
		MeetingURL:              req.MeetingURL,
		MeetingProv:             provider,
		BotName:                 req.BotName,
		BotImageURL:             req.BotImage,
		EntryMessage:            req.EntryMessage,
		RecordingMode:           req.RecordingMode,
		AllowMultipleBots:       allowMultiple,
		Extra:                   extraJSON,
		IdempotencyKey:          r.Header.Get("Idempotency-Key"),
		DeduplicationHash:       dedupHash,
		TranscriptionEnabled:    req.TranscriptionEnabled,
		StreamingEnabled:        req.StreamingEnabled,
		StreamingInputURL:       req.StreamingInputURL,
		StreamingOutputURL:      req.StreamingOutputURL,
		StreamingAudioFrequency: req.StreamingAudioFrequency,
		JoinAt:                  req.JoinAt,
		WebhookURL:              req.WebhookURL,
		Status:                  status,
	}
	if req.TranscriptionConfig != nil {
		row.TranscriptionProvider = req.TranscriptionConfig.Provider
	}
	if req.CallbackConfig != nil {
		row.CallbackURL = req.CallbackConfig.URL
	}

	if deps.Bots == nil {
		WriteError(w, http.StatusServiceUnavailable, CodeServiceUnavailable, "bot repository not configured")
		return
	}
	id, err := deps.Bots.InsertV2(r.Context(), row)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeInternal, "insert bot: "+err.Error())
		return
	}

	// Append a "queued" / "scheduled" outbox event for the webhook worker.
	if deps.Outbox != nil {
		eventCode := "bot.status_change"
		payload, _ := json.Marshal(map[string]any{
			"bot_id": id,
			"status": status,
		})
		_ = deps.Outbox.Append(r.Context(), tenantID, id, eventCode, payload)
	}

	// Enqueue onto the Redis stream so the controller can spawn a bot-worker.
	// Scheduled bots are not enqueued until their join_at time arrives.
	var streamID string
	if !scheduled && deps.Producer != nil {
		cfgPayload, _ := json.Marshal(map[string]any{
			"bot_uuid":         id,
			"user_id":          req.UserID,
			"meeting_url":      req.MeetingURL,
			"bot_name":         req.BotName,
			"recording_mode":   defaultStr(req.RecordingMode, "speaker_view"),
			"bots_webhook_url": req.WebhookURL,
			"automatic_leave":  buildTimeouts(req.TimeoutConfig),
			"environ":          "prod",
		})
		sid, err := deps.Producer.Enqueue(r.Context(), queue.Job{
			BotID:     id,
			BotUUID:   id,
			BotConfig: cfgPayload,
		})
		if err == nil {
			streamID = sid
		}
	}

	// Lock the dedup key now that the bot is persisted.
	if !allowMultiple && deps.Redis != nil {
		_ = deps.Redis.Set(r.Context(), dedupKey(tenantID, dedupHash), id, time.Hour).Err()
	}

	WriteCreated(w, CreateBotResponse{BotID: id, StreamID: streamID, Status: status})
}

// HandleGetBot serves GET /v2/bots/{bot_id}.
func HandleGetBot(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		if tenantID == "" {
			WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "tenant context missing")
			return
		}
		id := chi.URLParam(r, "bot_id")
		row, err := deps.Bots.GetV2(r.Context(), id)
		if errors.Is(err, postgres.ErrNotFound) {
			WriteError(w, http.StatusNotFound, CodeNotFound, "bot not found")
			return
		}
		if err != nil {
			WriteError(w, http.StatusInternalServerError, CodeInternal, err.Error())
			return
		}
		if row.TenantID != "" && row.TenantID != tenantID {
			WriteError(w, http.StatusNotFound, CodeNotFound, "bot not found")
			return
		}
		WriteOK(w, row)
	}
}

// HandleGetBotStatus serves GET /v2/bots/{bot_id}/status.
//
// Lightweight: first try Redis bot:state:<id>, fall back to a small PG
// projection.
func HandleGetBotStatus(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		if tenantID == "" {
			WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "tenant context missing")
			return
		}
		id := chi.URLParam(r, "bot_id")

		if deps.Redis != nil {
			if hash, err := deps.Redis.HGetAll(r.Context(), "bot:state:"+id).Result(); err == nil && len(hash) > 0 {
				WriteOK(w, hash)
				return
			}
		}

		row, err := deps.Bots.GetStatusLight(r.Context(), id)
		if errors.Is(err, postgres.ErrNotFound) {
			WriteError(w, http.StatusNotFound, CodeNotFound, "bot not found")
			return
		}
		if err != nil {
			WriteError(w, http.StatusInternalServerError, CodeInternal, err.Error())
			return
		}
		WriteOK(w, row)
	}
}

// HandleLeaveBot serves POST /v2/bots/{bot_id}/leave-bot.
func HandleLeaveBot(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		botID := chi.URLParam(r, "bot_id")
		if deps.Redis == nil {
			WriteError(w, http.StatusServiceUnavailable, CodeServiceUnavailable, "redis not configured")
			return
		}
		if err := queue.PublishStop(r.Context(), deps.Redis, botID, "apiRequest"); err != nil {
			WriteError(w, http.StatusInternalServerError, CodeInternal, err.Error())
			return
		}
		WriteAccepted(w, map[string]string{"status": "stop_signal_sent"})
	}
}

// HandlePauseBot serves POST /v2/bots/{bot_id}/pause-recording.
func HandlePauseBot(deps Deps) http.HandlerFunc {
	return botCmdHandler(deps, "pause", nil)
}

// HandleResumeBot serves POST /v2/bots/{bot_id}/resume-recording.
func HandleResumeBot(deps Deps) http.HandlerFunc {
	return botCmdHandler(deps, "resume", nil)
}

// ChatRequest is the body of POST /v2/bots/{bot_id}/chat-messages.
type ChatRequest struct {
	Text string `json:"text"`
}

// HandleSendChat serves POST /v2/bots/{bot_id}/chat-messages.
func HandleSendChat(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
			WriteError(w, http.StatusBadRequest, CodeInvalidParameters, "text is required")
			return
		}
		botCmdHandler(deps, "chat", map[string]any{"text": body.Text})(w, r)
	}
}

// botCmdHandler dispatches a generic command to the bot-worker via the
// bot:cmd:<id> pub/sub channel. The bot-worker subscribes to that channel
// (see internal/app/botworker.go) and routes the action.
func botCmdHandler(deps Deps, action string, extra map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Redis == nil {
			WriteError(w, http.StatusServiceUnavailable, CodeServiceUnavailable, "redis not configured")
			return
		}
		botID := chi.URLParam(r, "bot_id")
		payload := map[string]any{"action": action}
		for k, v := range extra {
			payload[k] = v
		}
		body, _ := json.Marshal(payload)
		if err := deps.Redis.Publish(r.Context(), "bot:cmd:"+botID, string(body)).Err(); err != nil {
			WriteError(w, http.StatusInternalServerError, CodeInternal, err.Error())
			return
		}
		WriteAccepted(w, map[string]string{"status": "command_sent", "action": action})
	}
}

// HandleDeleteData serves DELETE /v2/bots/{bot_id}/delete-data.
func HandleDeleteData(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		botID := chi.URLParam(r, "bot_id")
		if deps.Retention == nil {
			WriteError(w, http.StatusServiceUnavailable, CodeServiceUnavailable, "retention worker not configured")
			return
		}
		if err := deps.Retention.Schedule(r.Context(), tenantID, botID, time.Now()); err != nil {
			WriteError(w, http.StatusInternalServerError, CodeInternal, err.Error())
			return
		}
		WriteAccepted(w, map[string]string{"status": "delete_scheduled"})
	}
}

// HandleUsage serves GET /v2/usage.
func HandleUsage(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		if deps.Usage == nil {
			WriteOK(w, map[string]any{
				"tenant_id":    tenantID,
				"tokens_used":  0,
				"minutes_used": 0,
			})
			return
		}
		// Current calendar month window.
		now := time.Now().UTC()
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		sum, err := deps.Usage.Summarize(r.Context(), tenantID, start, end)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, CodeInternal, err.Error())
			return
		}
		WriteOK(w, sum)
	}
}

// HandleAlerts serves GET /v2/alerts.
func HandleAlerts(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		if deps.Alerts == nil {
			WriteOK(w, []any{})
			return
		}
		out, err := deps.Alerts.ListOpen(r.Context(), tenantID, 50)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, CodeInternal, err.Error())
			return
		}
		if out == nil {
			out = []postgres.AlertRow{}
		}
		WriteOK(w, out)
	}
}

// --- helpers --------------------------------------------------------------

func validateCreate(req *CreateBotRequest, scheduled bool) error {
	if strings.TrimSpace(req.MeetingURL) == "" {
		return errors.New("meeting_url is required")
	}
	if strings.TrimSpace(req.BotName) == "" {
		return errors.New("bot_name is required")
	}
	if scheduled && req.JoinAt == nil {
		return errors.New("join_at is required for scheduled bots")
	}
	if req.RecordingMode != "" {
		switch req.RecordingMode {
		case "speaker_view", "gallery_view", "audio_only":
		default:
			return fmt.Errorf("recording_mode must be one of speaker_view|gallery_view|audio_only")
		}
	}
	if req.TranscriptionEnabled && (req.TranscriptionConfig == nil || req.TranscriptionConfig.Provider == "") {
		return errors.New("transcription_config.provider is required when transcription_enabled=true")
	}
	if req.TimeoutConfig != nil {
		if t := req.TimeoutConfig.WaitingRoomTimeout; t != 0 && (t < 120 || t > 1800) {
			return errors.New("waiting_room_timeout must be between 120 and 1800")
		}
		if t := req.TimeoutConfig.NoOneJoinedTimeout; t != 0 && (t < 120 || t > 1800) {
			return errors.New("no_one_joined_timeout must be between 120 and 1800")
		}
		if t := req.TimeoutConfig.SilenceTimeout; t != 0 && (t < 300 || t > 1800) {
			return errors.New("silence_timeout must be between 300 and 1800")
		}
	}
	return nil
}

func detectProvider(u string) string {
	switch {
	case strings.Contains(u, "meet.google.com"):
		return "Meet"
	case strings.Contains(u, "teams.microsoft.com") || strings.Contains(u, "teams.live.com"):
		return "Teams"
	case strings.Contains(u, ".zoom.us/"):
		return "Zoom"
	default:
		return "unknown"
	}
}

func dedupHashFor(meetingURL string, joinAt *time.Time) string {
	mins := time.Now().UTC().Unix() / 60
	if joinAt != nil {
		mins = joinAt.UTC().Unix() / 60
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", meetingURL, mins)))
	return hex.EncodeToString(h[:])
}

func dedupKey(tenantID, hash string) string {
	return "dedup:" + tenantID + ":" + hash
}

// checkDedup returns (existingBotID, true) when a Redis dedup entry is found.
func checkDedup(ctx context.Context, rdb *goredisClient, tenantID, hash string) (string, bool) {
	if rdb == nil || hash == "" {
		return "", false
	}
	val, err := rdb.Get(ctx, dedupKey(tenantID, hash)).Result()
	if err != nil || val == "" {
		return "", false
	}
	return val, true
}

// goredisClient aliases redis.Client through this file so the import block
// stays compact when we only need the one type. Sets a clear seam if we ever
// want to swap to a smaller interface for tests.
type goredisClient = goredis.Client

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func buildTimeouts(t *TimeoutConfigReq) map[string]int {
	out := map[string]int{
		"waiting_room_timeout": 600,
		"noone_joined_timeout": 600,
		"silence_timeout":      600,
	}
	if t == nil {
		return out
	}
	if t.WaitingRoomTimeout > 0 {
		out["waiting_room_timeout"] = t.WaitingRoomTimeout
	}
	if t.NoOneJoinedTimeout > 0 {
		out["noone_joined_timeout"] = t.NoOneJoinedTimeout
	}
	if t.SilenceTimeout > 0 {
		out["silence_timeout"] = t.SilenceTimeout
	}
	return out
}
