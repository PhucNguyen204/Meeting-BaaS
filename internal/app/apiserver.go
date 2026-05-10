package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/config"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/queue"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/postgres"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
)

// APIServer is the runtime container for the api-server process.
//
// Phase 3: skeleton with POST /v1/bots that inserts into Postgres + enqueues
// onto Redis Streams. Real CRUD / auth / webhook dispatch comes later.
type APIServer struct {
	Logger *zap.Logger

	pg       *postgres.Pool
	rdb      *goredis.Client
	repo     *postgres.BotRepo
	producer *queue.Producer
	addr     string
	router   chi.Router
	httpSrv  *http.Server
}

// APIServerOptions tunes how NewAPIServer wires dependencies.
//
// Empty fields are filled from environment variables:
//
//	HTTP_ADDR     (default :8080)
//	POSTGRES_DSN  (required at Run time, may be empty at construct time)
//	REDIS_ADDR    (default localhost:6379)
//	REDIS_PASSWORD
//	QUEUE_STREAM  (default queue.DefaultStream)
type APIServerOptions struct {
	HTTPAddr      string
	PostgresDSN   string
	RedisAddr     string
	RedisPassword string
	QueueStream   string
}

// NewAPIServer constructs the api-server's dependency graph.
//
// Network connections (Postgres, Redis) are established eagerly so misconfig
// fails fast with a clear error before HTTP starts.
func NewAPIServer(log *zap.Logger, opts ...APIServerOptions) (*APIServer, error) {
	if log == nil {
		log = zap.NewNop()
	}
	var o APIServerOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	o = applyEnvDefaults(o)

	a := &APIServer{Logger: log, addr: o.HTTPAddr}

	if o.PostgresDSN != "" {
		pool, err := postgres.New(context.Background(), log, postgres.Options{DSN: o.PostgresDSN})
		if err != nil {
			return nil, fmt.Errorf("api-server: postgres: %w", err)
		}
		a.pg = pool
		a.repo = postgres.NewBotRepo(pool)
	} else {
		log.Warn("api-server: POSTGRES_DSN empty, /v1/bots will return 503")
	}

	if o.RedisAddr != "" {
		a.rdb = goredis.NewClient(&goredis.Options{
			Addr:     o.RedisAddr,
			Password: o.RedisPassword,
		})
		if err := a.rdb.Ping(context.Background()).Err(); err != nil {
			return nil, fmt.Errorf("api-server: redis ping: %w", err)
		}
		a.producer = queue.NewProducer(log, a.rdb, o.QueueStream)
	} else {
		log.Warn("api-server: REDIS_ADDR empty, jobs will not be enqueued")
	}

	a.router = a.buildRouter()
	return a, nil
}

// Run starts the HTTP server. Blocks until ctx is cancelled.
func (a *APIServer) Run(ctx context.Context) error {
	ctx = logger.IntoContext(ctx, a.Logger)
	log := logger.FromContext(ctx)
	log.Info("api-server starting", zap.String("addr", a.addr))

	a.httpSrv = &http.Server{
		Addr:              a.addr,
		Handler:           a.router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := a.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = a.httpSrv.Shutdown(shutdownCtx)
		a.closeDeps()
		return ctx.Err()
	case err := <-errCh:
		a.closeDeps()
		return err
	}
}

func (a *APIServer) closeDeps() {
	if a.pg != nil {
		a.pg.Close()
	}
	if a.rdb != nil {
		_ = a.rdb.Close()
	}
}

func (a *APIServer) buildRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", a.handleHealthz)
	r.Get("/readyz", a.handleHealthz)
	r.Post("/v1/bots", a.handleCreateBot)
	r.Get("/v1/bots/{id}", a.handleGetBot)
	r.Post("/v1/bots/{id}/stop", a.handleStopBot)
	return r
}

// --- Handlers ---

func (a *APIServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// createBotRequest is the JSON envelope api-server accepts. It mirrors a
// subset of MeetingParams + minimum fields the Phase 3 skeleton needs.
//
// The full BotConfig is forwarded as-is to the bot-worker via the queue.
type createBotRequest struct {
	BotUUID       string          `json:"bot_uuid"`
	UserID        int64           `json:"user_id"`
	MeetingURL    string          `json:"meeting_url"`
	BotName       string          `json:"bot_name"`
	WebhookURL    string          `json:"bots_webhook_url"`
	RecordingMode string          `json:"recording_mode"`
	BotConfig     json.RawMessage `json:"bot_config"` // optional pass-through
}

type createBotResponse struct {
	ID        string `json:"id"`
	BotUUID   string `json:"bot_uuid"`
	StreamID  string `json:"stream_id,omitempty"`
	Status    string `json:"status"`
}

func (a *APIServer) handleCreateBot(w http.ResponseWriter, r *http.Request) {
	if a.repo == nil || a.producer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "api-server: postgres or redis not configured"})
		return
	}

	var req createBotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	if req.MeetingURL == "" || req.BotName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "meeting_url and bot_name required"})
		return
	}

	provider := detectProviderName(req.MeetingURL)
	id, err := a.repo.Insert(r.Context(), postgres.BotRow{
		BotUUID:       defaultStr(req.BotUUID, req.BotUUID), // caller usually sets a UUID; pass-through
		UserID:        req.UserID,
		MeetingURL:    req.MeetingURL,
		MeetingProv:   provider,
		BotName:       req.BotName,
		RecordingMode: defaultStr(req.RecordingMode, string(config.RecModeSpeakerView)),
		WebhookURL:    req.WebhookURL,
		Status:        "queued",
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db insert: " + err.Error()})
		return
	}

	cfgPayload := req.BotConfig
	if len(cfgPayload) == 0 {
		// Build a minimal BotConfig payload from the request. The bot-worker
		// reads it from stdin.
		cfgPayload, _ = json.Marshal(map[string]any{
			"bot_uuid":         req.BotUUID,
			"user_id":          req.UserID,
			"meeting_url":      req.MeetingURL,
			"bot_name":         req.BotName,
			"recording_mode":   defaultStr(req.RecordingMode, string(config.RecModeSpeakerView)),
			"bots_webhook_url": req.WebhookURL,
			"automatic_leave": map[string]int{
				"waiting_room_timeout": 300,
				"noone_joined_timeout": 300,
				"silence_timeout":      300,
			},
			"environ": "prod",
		})
	}

	streamID, err := a.producer.Enqueue(r.Context(), queue.Job{
		BotID:     id,
		BotUUID:   req.BotUUID,
		BotConfig: cfgPayload,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "enqueue: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, createBotResponse{
		ID:       id,
		BotUUID:  req.BotUUID,
		StreamID: streamID,
		Status:   "queued",
	})
}

func (a *APIServer) handleGetBot(w http.ResponseWriter, r *http.Request) {
	if a.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "postgres not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	row, err := a.repo.Get(r.Context(), id)
	if errors.Is(err, postgres.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (a *APIServer) handleStopBot(w http.ResponseWriter, r *http.Request) {
	if a.rdb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "redis not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	row, err := a.repo.Get(r.Context(), id)
	if errors.Is(err, postgres.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := queue.PublishStop(r.Context(), a.rdb, row.BotUUID, "apiRequest"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stop_signal_sent"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// applyEnvDefaults fills empty fields from environment variables so callers
// don't have to plumb every var manually.
func applyEnvDefaults(o APIServerOptions) APIServerOptions {
	if o.HTTPAddr == "" {
		o.HTTPAddr = envOr("HTTP_ADDR", ":8080")
	}
	if o.PostgresDSN == "" {
		o.PostgresDSN = os.Getenv("POSTGRES_DSN")
	}
	if o.RedisAddr == "" {
		o.RedisAddr = envOr("REDIS_ADDR", "localhost:6379")
	}
	if o.RedisPassword == "" {
		o.RedisPassword = os.Getenv("REDIS_PASSWORD")
	}
	if o.QueueStream == "" {
		o.QueueStream = envOr("QUEUE_STREAM", queue.DefaultStream)
	}
	return o
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// detectProviderName is a thin re-implementation of the URL parser detector
// for the api-server's needs (the full version lives in infra/meeting and
// is bot-worker side).
func detectProviderName(meetingURL string) string {
	switch {
	case looksLikeMeet(meetingURL):
		return string(config.ProviderMeet)
	case looksLikeTeams(meetingURL):
		return string(config.ProviderTeams)
	case looksLikeZoom(meetingURL):
		return string(config.ProviderZoom)
	default:
		return "unknown"
	}
}

func looksLikeMeet(u string) bool  { return strings.Contains(u, "meet.google.com") }
func looksLikeTeams(u string) bool { return strings.Contains(u, "teams.microsoft.com") || strings.Contains(u, "teams.live.com") }
func looksLikeZoom(u string) bool  { return strings.Contains(u, ".zoom.us/") }

// ErrNotImplemented is kept exported for backward compatibility; some callers
// may still reference it. New code should not return it.
//
// Deprecated: use NewAPIServer + Run.
var ErrNotImplemented = errors.New("api-server: not implemented")

// IsConfigured reports whether the api-server has all required deps wired.
// Useful in startup scripts that want to verify before exposing /v1.
func (a *APIServer) IsConfigured() bool {
	return a != nil && a.repo != nil && a.producer != nil
}
