//go:build integration
// +build integration

// Run with: go test -tags=integration ./internal/infra/storage/postgres/...
//
// Requires Docker. The test spins up a transient Postgres 16 container,
// applies the migrations under meet-bot-go/migrations, and exercises BotRepo
// CRUD against it.
package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap/zaptest"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/postgres"
)

// TestBotRepoCRUD spins up Postgres in Docker, runs migrations, and verifies
// the Insert → Get → UpdateStatus loop end-to-end.
func TestBotRepoCRUD(t *testing.T) {
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
		// Apply each *.up.sql migration in lexicographic order on container init.
		tcpostgres.WithInitScripts(collectUpMigrations(t, migrationsDir)...),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("postgres testcontainer: %v", err)
	}
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(pgC)
	})

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := postgres.New(ctx, zaptest.NewLogger(t), postgres.Options{DSN: dsn})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	repo := postgres.NewBotRepo(pool)

	row := postgres.BotRow{
		BotUUID:       "11111111-1111-1111-1111-111111111111",
		UserID:        42,
		MeetingURL:    "https://meet.google.com/abc-defg-hij",
		MeetingProv:   "Meet",
		BotName:       "TestBot",
		RecordingMode: "speaker_view",
		WebhookURL:    "https://example.com/hook",
	}

	id, err := repo.Insert(ctx, row)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id == "" {
		t.Fatal("insert returned empty id")
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.BotName != row.BotName {
		t.Fatalf("expected BotName=%q, got %q", row.BotName, got.BotName)
	}
	if got.UserID != row.UserID {
		t.Fatalf("expected UserID=%d, got %d", row.UserID, got.UserID)
	}
	if got.MeetingProv != row.MeetingProv {
		t.Fatalf("expected MeetingProv=%q, got %q", row.MeetingProv, got.MeetingProv)
	}

	if err := repo.UpdateStatus(ctx, id, "in_call", "", ""); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got2, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got2.Status != "in_call" {
		t.Fatalf("expected Status=in_call, got %q", got2.Status)
	}
	if got2.JoinedAt == nil {
		t.Fatal("expected JoinedAt to be set after status=in_call")
	}

	// not-found path
	_, err = repo.Get(ctx, "00000000-0000-0000-0000-000000000000")
	if err == nil || err.Error() == "" {
		t.Fatal("expected not-found error")
	}
}

// collectUpMigrations returns the absolute paths of every *.up.sql file in
// migrationsDir in lexicographic order. The Postgres testcontainer applies
// these as init scripts inside the container.
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

// findMigrationsDir locates meet-bot-go/migrations relative to this test file.
// Walking up from runtime caller is more robust than relying on cwd, which
// differs between `go test ./...` and `go test ./internal/...`.
func findMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/infra/storage/postgres/integration_test.go → ../../../../migrations
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations"))
}
