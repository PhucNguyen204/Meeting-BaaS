//go:build integration
// +build integration

// Run with: go test -tags=integration ./internal/api/http/v2/...
//
// End-to-end happy-path: spin up Postgres + Redis via testcontainers, apply
// the migrations under meet-bot-go/migrations, seed a tenant + api key, fire
// POST /v2/bots and verify (a) the row landed in the bots table, (b) a job
// got XADD'd onto bots:jobs, and (c) GET /v2/bots/{id} returns it.
package v2_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap/zaptest"

	mw "github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/middleware"
	v2 "github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/v2"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/queue"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/postgres"
)

func TestCreateBotE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	migrationsDir := findMigrationsDir(t)

	pgC, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("meetbot_it"),
		tcpostgres.WithUsername("meetbot"),
		tcpostgres.WithPassword("meetbot"),
		tcpostgres.WithInitScripts(collectUpMigrations(t, migrationsDir)...),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("postgres testcontainer: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(pgC) })

	redisC, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("redis testcontainer: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(redisC) })

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	pool, err := postgres.New(ctx, zaptest.NewLogger(t), postgres.Options{DSN: dsn})
	if err != nil {
		t.Fatalf("pg pool: %v", err)
	}
	t.Cleanup(pool.Close)

	rAddr, err := redisC.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis addr: %v", err)
	}
	if u, err := goredis.ParseURL(rAddr); err == nil {
		rAddr = u.Addr
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: rAddr})
	t.Cleanup(func() { _ = rdb.Close() })

	// Seed a tenant + api key, then re-issue an API key whose plaintext we
	// know so we can hit the API.
	tenants := postgres.NewTenantRepo(pool)
	apiKeys := postgres.NewAPIKeyRepo(pool)
	tenantID, err := tenants.Insert(ctx, "Test Tenant", "test-tenant", "free", 7, nil)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	_, plaintext, err := apiKeys.Issue(ctx, tenantID, "", "it-key", []string{"bots:write", "bots:read"}, 600)
	if err != nil {
		t.Fatalf("seed api key: %v", err)
	}

	bots := postgres.NewBotRepo(pool)
	idem := postgres.NewIdempotencyRepo(pool)
	outbox := postgres.NewOutboxRepo(pool)
	producer := queue.NewProducer(zaptest.NewLogger(t), rdb, "bots:jobs:it")

	deps := v2.Deps{
		Logger:   zaptest.NewLogger(t),
		Bots:     bots,
		Outbox:   outbox,
		Producer: producer,
		Redis:    rdb,
	}

	// Wire a chi router that mirrors the production setup.
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(mw.Recover(zaptest.NewLogger(t)))
	r.Route("/v2", func(r chi.Router) {
		r.Use(mw.Auth(mw.AuthDeps{APIKeys: apiKeys, Redis: rdb}))
		r.Use(mw.RateLimit(rdb))
		v2.Mount(r, deps, mw.Idempotency(idem))
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	body := map[string]any{
		"meeting_url": "https://meet.google.com/abc-defg-hij",
		"bot_name":    "E2E Bot",
	}
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v2/bots", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plaintext)
	req.Header.Set("Idempotency-Key", "it-key-1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v2/bots: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
	var env v2.Envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	resp.Body.Close()
	if !env.Success {
		t.Fatalf("envelope.success=false: %+v", env.Error)
	}
	dataMap, _ := env.Data.(map[string]any)
	botID, _ := dataMap["bot_id"].(string)
	if botID == "" {
		t.Fatal("response missing bot_id")
	}

	// Idempotent replay: same Idempotency-Key returns cached response.
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/v2/bots", bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+plaintext)
	req2.Header.Set("Idempotency-Key", "it-key-1")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if resp2.Header.Get("Idempotent-Replayed") != "true" {
		t.Errorf("expected Idempotent-Replayed header on replay")
	}
	resp2.Body.Close()

	// Verify the row landed in postgres.
	row, err := bots.GetV2(ctx, botID)
	if err != nil {
		t.Fatalf("GetV2: %v", err)
	}
	if row.BotName != "E2E Bot" {
		t.Fatalf("bot_name mismatch: got %q", row.BotName)
	}
	if row.TenantID == "" {
		t.Error("tenant_id was not stamped")
	}

	// Verify a job was XADD'd.
	res, err := rdb.XLen(ctx, "bots:jobs:it").Result()
	if err != nil {
		t.Fatalf("xlen: %v", err)
	}
	if res < 1 {
		t.Fatalf("expected at least 1 job on bots:jobs:it, got %d", res)
	}

	// GET /v2/bots/{id} returns the bot.
	greq, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/bots/"+botID, nil)
	greq.Header.Set("Authorization", "Bearer "+plaintext)
	gresp, err := http.DefaultClient.Do(greq)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if gresp.StatusCode != http.StatusOK {
		t.Fatalf("GET want 200, got %d", gresp.StatusCode)
	}
	gresp.Body.Close()

	// /v2/usage returns an envelope (numbers may be zero, but shape is valid).
	ureq, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/usage", nil)
	ureq.Header.Set("Authorization", "Bearer "+plaintext)
	uresp, err := http.DefaultClient.Do(ureq)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if uresp.StatusCode != http.StatusOK {
		t.Fatalf("usage want 200, got %d", uresp.StatusCode)
	}
	uresp.Body.Close()
}

func collectUpMigrations(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	if len(out) == 0 {
		t.Fatalf("no *.up.sql migrations found in %s", dir)
	}
	return out
}

func findMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/api/http/v2/integration_test.go → ../../../../migrations
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations"))
}
