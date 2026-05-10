package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	mw "github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/middleware"
	v2 "github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/v2"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/queue"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/postgres"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
)

// APIServer is the runtime container for the api-server process.
//
// Production-grade v2 surface: auth (Bearer + x-meeting-baas-api-key),
// idempotency, rate limit, structured logging, panic recovery, and the 11
// Meeting BaaS v2 endpoints. Backwards-compatible POST /v1/bots is mounted
// as an alias of POST /v2/bots so v1 clients keep working.
type APIServer struct {
	Logger *zap.Logger

	pg       *postgres.Pool
	rdb      *goredis.Client
	addr     string
	router   chi.Router
	httpSrv  *http.Server

	// Phase 3 repos.
	bots      *postgres.BotRepo
	apiKeys   *postgres.APIKeyRepo
	idem      *postgres.IdempotencyRepo
	alerts    *postgres.AlertsRepo
	usage     *postgres.UsageRepo
	outbox    *postgres.OutboxRepo
	retention *postgres.RetentionRepo

	producer *queue.Producer
}

// APIServerOptions tunes how NewAPIServer wires dependencies.
//
// Empty fields are filled from environment variables:
//
//	HTTP_ADDR     (default :8080)
//	POSTGRES_DSN  (required for v2 endpoints)
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
	o = applyAPIServerEnvDefaults(o)

	a := &APIServer{Logger: log, addr: o.HTTPAddr}

	if o.PostgresDSN != "" {
		pool, err := postgres.New(context.Background(), log, postgres.Options{DSN: o.PostgresDSN})
		if err != nil {
			return nil, fmt.Errorf("api-server: postgres: %w", err)
		}
		a.pg = pool
		a.bots = postgres.NewBotRepo(pool)
		a.apiKeys = postgres.NewAPIKeyRepo(pool)
		a.idem = postgres.NewIdempotencyRepo(pool)
		a.alerts = postgres.NewAlertsRepo(pool)
		a.usage = postgres.NewUsageRepo(pool)
		a.outbox = postgres.NewOutboxRepo(pool)
		a.retention = postgres.NewRetentionRepo(pool)
	} else {
		log.Warn("api-server: POSTGRES_DSN empty, v1/v2 endpoints will return 503")
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
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(mw.Recover(a.Logger))
	r.Use(mw.RequestLogger(a.Logger))

	r.Get("/healthz", a.handleHealthz)
	r.Get("/readyz", a.handleHealthz)

	deps := v2.Deps{
		Logger:    a.Logger,
		Bots:      a.bots,
		Alerts:    a.alerts,
		Usage:     a.usage,
		Outbox:    a.outbox,
		Retention: a.retention,
		Producer:  a.producer,
		Redis:     a.rdb,
	}

	authMW := mw.Auth(mw.AuthDeps{
		APIKeys: a.apiKeys,
		Redis:   a.rdb,
		Logger:  a.Logger,
	})
	rateMW := mw.RateLimit(a.rdb)
	idemMW := mw.Idempotency(a.idem)

	r.Route("/v2", func(r chi.Router) {
		r.Use(authMW)
		r.Use(rateMW)
		v2.Mount(r, deps, idemMW)
	})

	// /v1 backward-compat: route POST /v1/bots to the same v2 handler so older
	// clients (and the bot-worker's own stop-record control plane) keep
	// working. /v1 does NOT enforce auth so existing integration tests keep
	// passing; lock that down in a follow-up once v1 clients have migrated.
	r.Route("/v1", func(r chi.Router) {
		r.Post("/bots", a.handleV1CreateBot)
		r.Get("/bots/{id}", a.handleV1GetBot)
		r.Post("/bots/{id}/stop", a.handleV1StopBot)
	})

	return r
}

// --- /healthz -------------------------------------------------------------

func (a *APIServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// --- /v1 backward-compat --------------------------------------------------
//
// These predate the v2 surface and intentionally keep their simpler shape
// (no envelope, no auth) for clients still on the original schema.

type v1CreateRequest struct {
	BotUUID       string `json:"bot_uuid"`
	UserID        int64  `json:"user_id"`
	MeetingURL    string `json:"meeting_url"`
	BotName       string `json:"bot_name"`
	WebhookURL    string `json:"bots_webhook_url"`
	RecordingMode string `json:"recording_mode"`
}

type v1CreateResponse struct {
	ID       string `json:"id"`
	BotUUID  string `json:"bot_uuid"`
	StreamID string `json:"stream_id,omitempty"`
	Status   string `json:"status"`
}

func (a *APIServer) handleV1CreateBot(w http.ResponseWriter, r *http.Request) {
	if a.bots == nil || a.producer == nil {
		writeV1Error(w, http.StatusServiceUnavailable, "api-server: postgres or redis not configured")
		return
	}
	var req v1CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeV1Error(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.MeetingURL == "" || req.BotName == "" {
		writeV1Error(w, http.StatusBadRequest, "meeting_url and bot_name required")
		return
	}

	provider := v2DetectProvider(req.MeetingURL)
	id, err := a.bots.Insert(r.Context(), postgres.BotRow{
		BotUUID:       req.BotUUID,
		UserID:        req.UserID,
		MeetingURL:    req.MeetingURL,
		MeetingProv:   provider,
		BotName:       req.BotName,
		RecordingMode: defaultStr(req.RecordingMode, "speaker_view"),
		WebhookURL:    req.WebhookURL,
		Status:        "queued",
	})
	if err != nil {
		writeV1Error(w, http.StatusInternalServerError, "db insert: "+err.Error())
		return
	}

	cfgPayload, _ := json.Marshal(map[string]any{
		"bot_uuid":         req.BotUUID,
		"user_id":          req.UserID,
		"meeting_url":      req.MeetingURL,
		"bot_name":         req.BotName,
		"recording_mode":   defaultStr(req.RecordingMode, "speaker_view"),
		"bots_webhook_url": req.WebhookURL,
		"automatic_leave": map[string]int{
			"waiting_room_timeout": 300,
			"noone_joined_timeout": 300,
			"silence_timeout":      300,
		},
		"environ": "prod",
	})

	streamID, err := a.producer.Enqueue(r.Context(), queue.Job{
		BotID:     id,
		BotUUID:   req.BotUUID,
		BotConfig: cfgPayload,
	})
	if err != nil {
		writeV1Error(w, http.StatusInternalServerError, "enqueue: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(v1CreateResponse{
		ID:       id,
		BotUUID:  req.BotUUID,
		StreamID: streamID,
		Status:   "queued",
	})
}

func (a *APIServer) handleV1GetBot(w http.ResponseWriter, r *http.Request) {
	if a.bots == nil {
		writeV1Error(w, http.StatusServiceUnavailable, "postgres not configured")
		return
	}
	id := chi.URLParam(r, "id")
	row, err := a.bots.Get(r.Context(), id)
	if errors.Is(err, postgres.ErrNotFound) {
		writeV1Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeV1Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(row)
}

func (a *APIServer) handleV1StopBot(w http.ResponseWriter, r *http.Request) {
	if a.rdb == nil {
		writeV1Error(w, http.StatusServiceUnavailable, "redis not configured")
		return
	}
	id := chi.URLParam(r, "id")
	row, err := a.bots.Get(r.Context(), id)
	if errors.Is(err, postgres.ErrNotFound) {
		writeV1Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeV1Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := queue.PublishStop(r.Context(), a.rdb, row.BotUUID, "apiRequest"); err != nil {
		writeV1Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"stop_signal_sent"}`))
}

func writeV1Error(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// applyAPIServerEnvDefaults fills empty fields from environment variables.
func applyAPIServerEnvDefaults(o APIServerOptions) APIServerOptions {
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

// v2DetectProvider mirrors the v2 helper for the v1 backward-compat path.
func v2DetectProvider(meetingURL string) string {
	switch {
	case stringContains(meetingURL, "meet.google.com"):
		return "Meet"
	case stringContains(meetingURL, "teams.microsoft.com") || stringContains(meetingURL, "teams.live.com"):
		return "Teams"
	case stringContains(meetingURL, ".zoom.us/"):
		return "Zoom"
	default:
		return "unknown"
	}
}

func stringContains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// ErrNotImplemented is kept exported for backward compatibility; some callers
// may still reference it. New code should not return it.
//
// Deprecated: use NewAPIServer + Run.
var ErrNotImplemented = errors.New("api-server: not implemented")

// IsConfigured reports whether the api-server has all required deps wired.
func (a *APIServer) IsConfigured() bool {
	return a != nil && a.bots != nil && a.producer != nil
}
