# Makefile for meet-bot-go
#
# Common entrypoints. Run `make help` for a complete list.

SHELL          := /bin/bash
MODULE         := github.com/yourorg/meet-bot-go
BIN_DIR        := bin
GO             ?= go
GOFLAGS        ?=
LDFLAGS_VERSION := -X $(MODULE)/internal/pkg/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev) \
                   -X $(MODULE)/internal/pkg/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown) \
                   -X $(MODULE)/internal/pkg/version.BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

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
	rm -rf $(BIN_DIR)

.PHONY: docker-bot-worker
docker-bot-worker: ## Build bot-worker docker image
	docker build -f deployments/docker/bot-worker.Dockerfile -t meet-bot-go/bot-worker:dev .

.PHONY: docker-compose-up
docker-compose-up: ## Start dev infra (redis, postgres, minio)
	docker compose -f deployments/docker/docker-compose.dev.yml up -d

.PHONY: docker-compose-down
docker-compose-down:
	docker compose -f deployments/docker/docker-compose.dev.yml down -v
