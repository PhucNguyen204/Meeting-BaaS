# API Server Reference

> Self-hosted Meeting BaaS v2 REST surface served by `cmd/api-server`.
> Contract derived from [Meeting BaaS v2 reference](https://docs.meetingbaas.com/docs/api-v2/reference); see also [docs/database-design.md](database-design.md).

The api-server fronts the data plane: it authenticates requests, validates them, persists into Postgres, optionally enqueues jobs onto Redis Streams (the `controller` then spawns a `bot-worker`), and exposes lifecycle commands routed back to running bots via Redis Pub/Sub.

---

## Mục lục

1. [Authentication](#1-authentication)
2. [Idempotency](#2-idempotency)
3. [Rate limiting](#3-rate-limiting)
4. [Response envelope and error codes](#4-response-envelope-and-error-codes)
5. [Endpoint reference](#5-endpoint-reference)
6. [Webhooks](#6-webhooks)
7. [End-to-end examples](#7-end-to-end-examples)
8. [Differences vs upstream Meeting BaaS](#8-differences-vs-upstream-meeting-baas)
9. [Running the server](#9-running-the-server)

---

## 1. Authentication

Every `/v2/**` endpoint requires an API key. The api-server accepts two transports (Meeting BaaS upstream compat):

```
Authorization: Bearer mb_<32-hex>
x-meeting-baas-api-key: mb_<32-hex>
```

API keys are minted via `APIKeyRepo.Issue` (see `internal/infra/storage/postgres/api_key_repo.go`). Plaintext starts with `mb_` followed by 32 hex chars; we store the sha256 hash and an 8-character prefix for UI display.

The auth middleware first checks Redis (`apikey:cache:<sha256-hex>`, TTL 5 min). On miss it falls back to Postgres and back-fills the cache. Revoked / expired keys are filtered server-side; a 401 is returned for any failure case so callers cannot distinguish "wrong key" from "revoked".

On a successful auth the request context carries:
- `tenant_id`
- `api_key_id`
- `key_prefix` (logging friendly)
- `rate_limit_per_min`

These are read by downstream middleware and handlers via the helpers in [`internal/api/http/middleware/context.go`](../internal/api/http/middleware/context.go).

---

## 2. Idempotency

POST / PUT / DELETE endpoints honor an `Idempotency-Key` header (any opaque string up to 255 chars). The middleware records the request body's SHA-256 in `idempotency_keys` keyed by `(tenant_id, idempotency_key)` and short-circuits replays:

| Situation | Server response |
|---|---|
| First call with that key | Run handler, capture status + body, store in `idempotency_keys`. |
| Replay, same request body | Return cached status + body. `Idempotent-Replayed: true` header set. |
| Replay, different body | `409 IDEMPOTENCY_CONFLICT`. |
| Replay while first is still in flight | `409 CONFLICT` with "still being processed". |

Idempotency rows expire after 24h (cron handled by `IdempotencyRepo.SweepExpired`).

---

## 3. Rate limiting

Per-key per-minute quota lives on `api_keys.rate_limit_per_min` (default `600`). The middleware atomically `INCR`s `rate:{api_key_id}:{minute_window}` in Redis with a 65s TTL. On exceed:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 23
X-RateLimit-Limit: 600
X-RateLimit-Remaining: 0

{
  "success": false,
  "error": { "code": "RATE_LIMITED", "message": "rate limit exceeded (600/min); retry in 23s" }
}
```

Successful requests carry `X-RateLimit-Limit` and `X-RateLimit-Remaining` so clients can pace themselves. Redis outages fail open (we'd rather over-allow briefly than 500 the whole API).

---

## 4. Response envelope and error codes

All `/v2/**` responses use the standardized envelope ([`internal/api/http/respond/respond.go`](../internal/api/http/respond/respond.go)):

```json
{ "success": true,  "data":  { ... } }
{ "success": false, "error": { "code": "...", "message": "...", "details": {...} } }
```

Error codes:

| Code | HTTP | Meaning |
|---|---|---|
| `INVALID_PARAMETERS` | 400 | Validation failure; `details` may carry field errors |
| `UNAUTHORIZED` | 401 | Missing / invalid API key |
| `FORBIDDEN` | 403 | Authenticated but lacks scope |
| `NOT_FOUND` | 404 | Resource not in scope |
| `CONFLICT` | 409 | General conflict (e.g. dedup) |
| `IDEMPOTENCY_CONFLICT` | 409 | Idempotency-Key reused with different body |
| `RATE_LIMITED` | 429 | Per-key per-minute quota exceeded |
| `INSUFFICIENT_TOKENS` | 402 | Account out of tokens (Phase 6) |
| `INTERNAL` | 500 | Unexpected server error |
| `SERVICE_UNAVAILABLE` | 503 | Postgres / Redis dependency missing |

---

## 5. Endpoint reference

All paths are `/v2/...` unless otherwise noted. `Authorization: Bearer mb_...` header required for every endpoint except `/healthz` and `/readyz`.

### `GET /healthz`, `GET /readyz`

Liveness / readiness probes. Returns `{"status":"ok"}` with HTTP 200. Not enveloped — kept stable for K8s probes.

### `POST /v2/bots` — create an immediate bot

Request body (matches the public Meeting BaaS v2 schema):

```json
{
  "meeting_url": "https://meet.google.com/abc-defg-hij",
  "bot_name": "AI Notetaker",
  "bot_image": "https://example.com/bot.png",
  "entry_message": "Hi team, recording started.",
  "recording_mode": "speaker_view",
  "transcription_enabled": true,
  "transcription_config": { "provider": "gladia", "api_key": "..." },
  "streaming_enabled": false,
  "streaming_input": "wss://...",
  "streaming_output": "wss://...",
  "streaming_audio_frequency": 24000,
  "timeout_config": {
    "waiting_room_timeout": 600,
    "no_one_joined_timeout": 600,
    "silence_timeout": 600
  },
  "allow_multiple_bots": true,
  "extra": { "external_id": "deal-42" },
  "callback_config": { "url": "https://example.com/hook", "secret": "shh" },
  "bots_webhook_url": "https://example.com/account-webhook"
}
```

Required: `meeting_url`, `bot_name`. `transcription_enabled=true` requires `transcription_config.provider`. Timeouts must be within Meeting BaaS-spec ranges (120-1800s; silence 300-1800s).

Response: `201 Created`:

```json
{
  "success": true,
  "data": {
    "bot_id": "...",
    "stream_id": "1715-0",
    "status": "queued"
  }
}
```

Side effects: bot row inserted (status `queued`), webhook outbox event `bot.status_change` appended, job XADD'd onto `bots:jobs`.

### `POST /v2/bots/scheduled` — schedule a future bot

Same body as immediate, but `join_at` (RFC 3339 UTC) is required. Status starts as `scheduled`; no Redis enqueue until a scheduler (Phase 5) wakes the row at `join_at`.

### `GET /v2/bots/{bot_id}` — full bot details

Returns the v2 row (transcription / streaming / extras / S3 keys when available). Tenant-scoped: 404 if the bot belongs to a different tenant.

### `GET /v2/bots/{bot_id}/status` — lightweight status

Tries Redis `bot:state:<id>` first (a running bot-worker heartbeats here every 5s). Falls back to a small Postgres projection (status + timestamps). Useful for dashboards that poll.

### `POST /v2/bots/{bot_id}/leave-bot` — force leave

Publishes `bot:stop:<id>` Pub/Sub message with reason `apiRequest`. The running bot-worker subscribes and transitions to Cleanup. Response 202.

### `POST /v2/bots/{bot_id}/pause-recording` / `POST /v2/bots/{bot_id}/resume-recording`

Publish `bot:cmd:<id>` Pub/Sub messages `{"action":"pause"}` / `{"action":"resume"}`. The bot-worker subscribes (see [`internal/app/botworker.go`](../internal/app/botworker.go) `subscribeBotCommands`) and toggles `MeetingContext.IsPaused`. Response 202.

### `POST /v2/bots/{bot_id}/chat-messages` — send chat to meeting

Body: `{ "text": "Hello from the bot" }`. Published as `bot:cmd:<id>` `{"action":"chat","text":"..."}`. Bot-worker dispatches to `meet.SendEntryMessage`. Response 202.

### `DELETE /v2/bots/{bot_id}/delete-data` — opt-in retention

Schedules a `data_retention_jobs` row with `scheduled_at = NOW()`. A retention worker (Phase 6) drains the queue, deletes the S3 prefix, purges Mongo transcripts, and stamps `bots.data_deleted_at`. Response 202.

### `GET /v2/usage` — current period usage

Returns this calendar month's aggregated `usage_records` (tokens, minutes, total cost). Returns zeros if no Usage repo is configured (MVP).

### `GET /v2/alerts` — operational alerts

Returns up to 50 open `alerts` for the tenant, newest first.

---

## 6. Webhooks

Two webhook surfaces (matching Meeting BaaS v2):

1. **Account-level**: configured in `webhook_endpoints`. Receives every event for every bot in that tenant, signed with the SVIX-style headers (`svix-id`, `svix-timestamp`, `svix-signature`). Implementation lands in Phase 4.
2. **Per-bot callback**: passed inline via `callback_config.url` on bot creation. Receives only `bot.completed` / `bot.failed`. Secret stored in `secrets` table via envelope encryption.

Events are recorded transactionally into `webhook_event_outbox` alongside the bot mutation that triggered them. A separate delivery worker (Phase 4) drains the outbox with `FOR UPDATE SKIP LOCKED` and writes attempt rows into `webhook_deliveries`.

Bot event lifecycle codes (subset, matching Meeting BaaS):

```
queued -> scheduled? -> joining_call -> in_waiting_room? ->
in_call_not_recording -> in_call_recording ->
(recording_paused -> recording_resumed)? ->
call_ended -> completed | failed
```

---

## 7. End-to-end examples

### Create and poll an immediate bot

```bash
TOKEN="mb_..."

curl -s -X POST https://api.example.com/v2/bots \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: bot-001" \
  -d '{
    "meeting_url": "https://meet.google.com/abc-defg-hij",
    "bot_name": "Notetaker",
    "recording_mode": "speaker_view",
    "transcription_enabled": true,
    "transcription_config": { "provider": "gladia" }
  }' | jq

# Poll lightweight status while the bot runs
BOT_ID=$(curl -s -H "Authorization: Bearer $TOKEN" ... | jq -r '.data.bot_id')
while true; do
  curl -s -H "Authorization: Bearer $TOKEN" \
    https://api.example.com/v2/bots/$BOT_ID/status | jq
  sleep 5
done
```

### Pause, resume, send a chat message

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  https://api.example.com/v2/bots/$BOT_ID/pause-recording

curl -X POST -H "Authorization: Bearer $TOKEN" \
  https://api.example.com/v2/bots/$BOT_ID/resume-recording

curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text":"Heads up — I am pausing for 5 minutes."}' \
  https://api.example.com/v2/bots/$BOT_ID/chat-messages
```

### Go SDK call

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

func main() {
    body, _ := json.Marshal(map[string]any{
        "meeting_url":   "https://meet.google.com/abc-defg-hij",
        "bot_name":      "Notetaker",
        "recording_mode": "speaker_view",
    })
    req, _ := http.NewRequest("POST", "https://api.example.com/v2/bots", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer mb_...")
    req.Header.Set("Idempotency-Key", "bot-001")
    req.Header.Set("Content-Type", "application/json")
    _, _ = http.DefaultClient.Do(req)
}
```

---

## 8. Differences vs upstream Meeting BaaS

Not yet implemented in this build:

- **Calendar integration** (`/v2/calendars/**`): Phase 5.5.
- **Real webhook delivery** (signing, retries, auto-disable): Phase 4. Outbox is wired; delivery worker is not.
- **SVIX-style signature verification**: Phase 6.
- **Stripe billing integration**: Phase 7.
- **Screenshots endpoint** (`GET /v2/bots/{id}/screenshots`): planned with MongoDB `screenshots` collection.
- **Account-level RLS**: schema supports it (every table has `tenant_id`), middleware sets `app.tenant_id`, but per-request `SET LOCAL` on the pgx pool needs Phase 5 to fully enforce.
- **MongoDB transcripts**: schema and contract in [database-design.md](database-design.md); collections will be wired when the transcription provider integration lands.

Backward compatibility: `POST /v1/bots`, `GET /v1/bots/{id}`, `POST /v1/bots/{id}/stop` remain available without auth so older clients keep working. These will be deprecated and removed in v2-only.

---

## 9. Running the server

```bash
# Apply migrations (one-time)
DB_URL="postgres://postgres:postgres@localhost:5432/meetbot?sslmode=disable" make migrate-up

# Start dependencies
make dev-up   # docker compose: postgres + redis + minio + mailhog

# Build + run
make build-api-server
HTTP_ADDR=:8080 \
POSTGRES_DSN=postgres://meetbot:meetbot@localhost:5432/meetbot?sslmode=disable \
REDIS_ADDR=localhost:6379 \
QUEUE_STREAM=bots:jobs \
LOG_LEVEL=debug \
bin/api-server
```

Environment variables consumed by `api-server`:

| Variable | Default | Purpose |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Listen address |
| `POSTGRES_DSN` | (required) | pgx connection string |
| `REDIS_ADDR` | `localhost:6379` | Redis host:port |
| `REDIS_PASSWORD` | (empty) | Redis auth |
| `QUEUE_STREAM` | `bots:jobs` | Redis Stream key |
| `LOG_LEVEL` | `info` | zap level (debug/info/warn/error) |

To seed an initial tenant + API key for local testing:

```sql
INSERT INTO tenants (name, slug, plan) VALUES ('Dev', 'dev', 'free') RETURNING id;
-- then in Go (or psql) issue an API key via APIKeyRepo.Issue and surface the plaintext once.
```

The integration tests in [`internal/api/http/v2/integration_test.go`](../internal/api/http/v2/integration_test.go) (run via `make test-integration`) exercise the full flow end-to-end against ephemeral Postgres + Redis containers.
