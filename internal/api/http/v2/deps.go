package v2

import (
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/queue"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/postgres"
)

// Deps bundles every dependency the v2 handlers need so the router wiring
// stays a single line per route.
//
// Tests instantiate Deps directly with stub fields they care about.
type Deps struct {
	Logger *zap.Logger

	Bots          *postgres.BotRepo
	Alerts        *postgres.AlertsRepo
	Usage         *postgres.UsageRepo
	Outbox        *postgres.OutboxRepo
	Retention     *postgres.RetentionRepo

	Producer *queue.Producer
	Redis    *redis.Client
}
