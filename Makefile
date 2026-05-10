# Makefile for meet-bot-go
#
# Common entrypoints. Run `make help` for a complete list.

SHELL          := /bin/bash
MODULE         := github.com/PhucNguyen204/Meeting-BaaS
BIN_DIR        := bin
GO             ?= go
GOFLAGS        ?=
LDFLAGS_VERSION := -X $(MODULE)/internal/pkg/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev) \
                   -X $(MODULE)/internal/pkg/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown) \
                   -X $(MODULE)/internal/pkg/version.BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Database / migrations (Phase 3)
MIGRATE        ?= $(GO) run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate
DB_URL         ?= postgres://postgres:postgres@localhost:5432/meetbot?sslmode=disable
MIGRATIONS_DIR := migrations

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: deps
deps: ## go mod tidy + install playwright browsers
	$(GO) mod tidy
	$(GO) run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps chromium

.PHONY: build
build: build-bot-worker build-api-server build-controller ## Build all binaries

.PHONY: build-bot-worker
build-bot-worker: ## Build bot-worker
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS_VERSION)" -o $(BIN_DIR)/bot-worker ./cmd/bot-worker

.PHONY: build-api-server
build-api-server: ## Build api-server (stub)
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS_VERSION)" -o $(BIN_DIR)/api-server ./cmd/api-server

.PHONY: build-controller
build-controller: ## Build controller (stub)
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS_VERSION)" -o $(BIN_DIR)/controller ./cmd/controller

.PHONY: run
run: build-bot-worker ## Run bot-worker locally with example config
	LOG_LEVEL=debug DEBUG_LOGS=true $(BIN_DIR)/bot-worker < configs/bot.config.example.json

.PHONY: test
test: ## Run unit tests
	$(GO) test -race -count=1 ./...

.PHONY: test-short
test-short: ## Run unit tests, skip integration (-short)
	$(GO) test -race -count=1 -short ./...

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker)
	$(GO) test -race -count=1 -tags=integration -timeout 10m ./internal/infra/storage/postgres/... ./internal/infra/queue/... ./internal/infra/storage/s3/...

.PHONY: cover
cover: ## Generate coverage profile (excludes integration tests)
	$(GO) test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

.PHONY: vet
vet: ## go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## golangci-lint run
	$(GO) run github.com/golangci/golangci-lint/cmd/golangci-lint run ./...

.PHONY: fmt
fmt: ## gofmt + goimports
	$(GO) fmt ./...

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf $(BIN_DIR) coverage.out coverage.html

# --- Database migrations (Phase 3) ----------------------------------------
.PHONY: migrate-up
migrate-up: ## Apply all up migrations (DB_URL=postgres://...)
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back one migration
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down 1

.PHONY: migrate-create
migrate-create: ## Create a new migration file (NAME=add_foo)
	@test -n "$(NAME)" || (echo "usage: make migrate-create NAME=add_foo" && exit 1)
	$(MIGRATE) create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME)

# --- Docker --------------------------------------------------------------
.PHONY: docker-bot-worker
docker-bot-worker: ## Build bot-worker docker image
	docker build -f deployments/docker/bot-worker.Dockerfile -t meet-bot-go/bot-worker:dev .

.PHONY: docker-compose-up
docker-compose-up: dev-up ## Alias of dev-up
.PHONY: dev-up
dev-up: ## Start dev infra (redis, postgres, minio, mailhog)
	docker compose -f deployments/docker/docker-compose.dev.yml up -d

.PHONY: docker-compose-down
docker-compose-down: dev-down ## Alias of dev-down
.PHONY: dev-down
dev-down: ## Tear down dev infra
	docker compose -f deployments/docker/docker-compose.dev.yml down -v
