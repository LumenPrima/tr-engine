# Quality Gates

This baseline applies to every `tr-engine` implementation issue. An issue is not complete until it names the commands or manual checks it used, or documents why a gate could not run.

## Required Local Gates

Run these before opening review for backend changes:

```bash
bash build.sh
go test ./...
go vet ./...
```

For a narrower inner loop, run package-level tests first, for example:

```bash
go test ./internal/api
go test ./internal/ingest
go test ./internal/audio
```

The final verification for shared backend work should still include `go test ./...` unless a blocker is documented.

## API Contract Gates

`openapi.yaml` is the source of truth for the public REST, SSE, and live-audio contract. Any change that adds, removes, renames, or changes behavior for an endpoint, request, response, schema, auth requirement, SSE event, or audio route must include:

- The corresponding `openapi.yaml` update in the same change.
- Focused endpoint tests in `internal/api/*_test.go` for status code, auth behavior, error shape, and response body.
- Regeneration or downstream verification for clients that consume the contract, especially `tr-dashboard`'s `npm run api:generate` when dashboard types are affected.
- A diff check that the OpenAPI change matches the implementation and does not remove unrelated paths or schemas.

If a local OpenAPI validator is available, run it and include the command in the verification block. Until a validator command is committed, the minimum contract gate is implementation tests plus review of the `openapi.yaml` diff.

## Backend Test Expectations

Add or update tests for:

- New or changed HTTP endpoints, middleware, auth branches, API key behavior, pagination, filtering, and error responses.
- SSE event publishing, filtering, replay, reconnect assumptions, and event payload shape.
- Live audio ingest, routing, deduplication, and encoding behavior, especially failures that should surface to clients.
- Ingest handlers, identity resolution, deduplication, backfill behavior, and file or MQTT parsing.
- Database query behavior when SQL, migrations, or sqlc-generated code changes.

Prefer focused package tests for the changed boundary, then run the broader suite before completion.

## Manual Verification Checklist

Use the smallest checklist that covers the changed behavior. Include the checked items in the issue or PR.

- Auth: verify open, token, and full JWT modes relevant to the change; failed auth must return the documented error shape.
- SSE: verify `/api/v1/events/stream` emits the changed event, honors filters, sends replay IDs, and behaves through the intended reverse proxy without buffering.
- Live audio: verify `/audio/live` upgrade behavior, subscription messages, disabled-stream errors, and client-visible close/error behavior.
- Reverse proxy: verify `/api/*`, `/audio/*`, `/health/*`, `/docs.html`, and `/openapi.yaml` route correctly in the deployment shape being changed.
- OpenAPI: verify Swagger UI or `/openapi.yaml` exposes the updated contract.
- Dashboard compatibility: verify affected dashboard calls compile or regenerate cleanly when the contract changes.

## Observability Baseline

New operationally significant work should leave enough information to diagnose failures without reproducing the full environment:

- Structured logs with stable event names and useful IDs for auth, ingest, SSE, live audio, transcription, storage, and proxy-facing failures.
- User-visible error responses using the documented API error envelope instead of opaque 500s where the failure is expected.
- Health or debug-report data for connection state, stream status, build/version metadata, and recent subsystem errors when practical.
- Metrics or counters for high-volume paths where regressions are otherwise invisible, such as ingest drops, SSE subscribers, live-audio clients, queue depth, and transcription failures.
- Debug report links or collected fields that let maintainers correlate browser reports with backend logs.

## Completion Rule

Every implementation issue should end with a verification block like:

```text
Verification:
- bash build.sh
- go test ./...
- go vet ./...
- Manual: /api/v1/events/stream emits call_end through Caddy with buffering disabled.
```

If a gate cannot run, record the blocker, the risk, and the narrower check that was run instead.
