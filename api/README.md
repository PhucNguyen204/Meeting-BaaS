# api/

Schema-first definitions go here. Phase 3 deliverables:

- `proto/bot/v1/*.proto` — gRPC contract for the (currently unused)
  controller -> bot-worker channel.
- `openapi/api-server.yaml` — OpenAPI 3.1 description of the public
  REST API (queueing jobs, listing recordings, webhook contracts).

Code generated from these definitions lands in `internal/generated/` and
is committed to the repo (so `go build` works without a separate codegen
step). Generators are pinned in `tools/tools.go`.
