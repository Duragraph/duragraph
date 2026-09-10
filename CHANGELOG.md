# Changelog

All notable changes to DuraGraph will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.8.1] - 2026-09-09

### Fixed

- **The rebuilt control plane is now reachable from the binary.** It was
  not in 0.8.0. The release pipeline builds `./cmd/duragraph`, and that
  command imported none of `controlplane/` — so everything 0.8.0's notes
  described shipped as unreachable code while the binary ran the previous
  implementation. See the correction on 0.8.0 below.

  Select it with `duragraph serve --control-plane=v2`, or
  `DURAGRAPH_CONTROL_PLANE=v2`. **The default remains the legacy stack**,
  because the rebuild does not yet serve every route the legacy one does
  — password authentication (`POST /api/auth/register`, `/login`),
  `GET /health`, `POST /mcp`, and the assistant schema/subgraph endpoints
  are still legacy-only. Those are being ported before the default moves.

- **The control plane can now restart.** Its migrations used plain
  `CREATE TABLE` with nothing recording which had run, so a second start
  against the same database failed with `relation already exists` and the
  process refused to boot — it could be started exactly once per
  database. Migrations are now tracked in `schema_migrations`, and each
  applies in one transaction with the row recording it.

- **Migrations are embedded in the binary.** They were read from a
  source-tree path resolved against the working directory, so an
  installed binary could not migrate at all.

- The API container image now builds. Its Dockerfile copies an allowlist
  of source trees rather than the whole context, and `controlplane/` was
  not on it.

## [0.8.0] - 2026-09-09

**Correction (0.8.1).** The entry below describes the rebuilt control
plane, and every word of it is true of the `controlplane/` packages. It
was misleading about the released artifact: the rebuild was **not wired
into the `duragraph` binary in 0.8.0**, so a user who installed and ran
it got the previous implementation, in which `llm.token`,
`checkpoint.saved` and the SSE heartbeat are absent, cron advances its
schedule without creating any run, and completed runs do not record their
output. The wiring landed in 0.8.1 behind `--control-plane=v2`. The
original text is kept below unaltered rather than quietly edited.

Note: this entry resumes a changelog that had gone quiet — 0.7.1 through
0.7.7 were tagged and released without entries here. Their contents are
in the GitHub release notes for those tags.

The control plane is rebuilt against the structural specification, and a
single user can now drive a graph end to end over the HTTP API alone:
install a graph, create an assistant and thread, start a run, watch it
stream, pause it for human input, resume it, and read the result back.

### Added

- **Control-plane rebuild** — layers 1–4, event store, and system
  endpoints, replacing the previous implementation.
- **Graph execution engine** — edge-driven walk over nodes, with durable
  checkpoints and resume after redelivery.
- **Graph installation over the API.** `POST /workers/register` accepts
  `graph_definitions` and upserts them in the same transaction. Before
  this there was no way to install a graph over HTTP at all — both graph
  routes were reads.
- **Human-in-the-loop interrupts.** `interrupt_before` / `interrupt_after`
  (graph-level and per-run), plus `requires_human`, `human`, and
  `tool_calls` triggers. Resume applies the full `Command` (`resume` and
  `goto`), not just a state update.
- **Run streaming (SSE) and wait.** All twelve stream events the API
  specification declares are now emitted: `run.*`, `execution.node_started`
  / `node_completed` / `node_failed`, `checkpoint.saved`, `tool.call`,
  `tool.result`, `llm.completion`, `llm.token`, and a periodic
  `heartbeat` so idle streams survive intermediary timeouts.
- **`llm.token` streams over an ephemeral path** — at-most-once, never
  persisted, no replay. Tokens are superseded by the completion seconds
  later, so persisting one row per token would multiply write volume by
  the length of every generation to store data nobody reads twice. The
  durable events endpoint rejects the type outright to keep it that way.
- **LLM and tool nodes delegate to sub-workers** over NATS request/reply,
  behind a provider seam. Streaming is an optional provider capability;
  providers that cannot stream need no changes.
- **Run results.** `runs.output` is frozen from the final channels when a
  run completes, and `GET /threads/{id}/state` now unwraps the worker
  checkpoint envelope, so `values` holds the channels and `next` reports
  where a paused run will resume.
- **Cron scheduling** that actually fires — a ticker with
  `FOR UPDATE SKIP LOCKED`, one transaction per cron. Invalid schedules
  are rejected at create time and retired at fire time.
- **Assistant version history** — versions are snapshotted on create and
  update, with read and rollback endpoints.
- **Idempotent creates.** `assistant_id` / `thread_id` with `if_exists`,
  and graph-name resolution for `assistant_id` on run and cron create.
- **Tenant CRUD** and the platform surface (auth, admin, users).
- **Run reaper** — runs stuck past the redelivery window are failed
  rather than left hanging.
- **Direct NATS + JetStream** replaces watermill, with a transactional
  outbox in application code driven by `LISTEN`/`NOTIFY`.
- **Simplified Chinese README** (`README.zh-CN.md`).

### Fixed

- Worker reliability — a relay shutdown race, and dead-letter and
  graph-error paths that failed to mark the run failed.
- Thread and checkpoint identifiers are validated at the API boundary,
  returning 422 rather than surfacing a database error.
- A graph registered by name was invisible to lookup, which resolved only
  by assistant binding. Both bindings the schema declares are now honored.
- `execution_history.node_type` accepts `human`.

### Testing

- Integration tests run against real Postgres and NATS via testcontainers,
  in CI as well as locally, plus a Tier-2 regression suite over the real
  engine.

### Email + Password Authentication (Phase A)

- **`POST /api/auth/register`** — email + password signup. Returns 201 on
  success, 409 on duplicate, 400 on short/long password. The first user to
  register is auto-elevated to admin + approved (bootstrap branch — same
  semantics as the OAuth bootstrap).
- **`POST /api/auth/login`** — email + password login. Returns 200 with
  session cookie + JWT on success. ALL failure modes (unknown email, wrong
  password, suspended/pending account) collapse to a uniform 401 with
  message "Invalid email or password" — by design, to prevent account-state
  enumeration. See `auth/password.yml` § generic_401.
- **bcrypt cost 12** for password hashing; tests use `bcrypt.MinCost`.
- **Migration `005_user_password.sql`** — adds `password_hash`, `auth_method`
  to `platform.users`; `oauth_provider`/`oauth_id` become NULLABLE; CHECK
  constraint requires at least one auth method per row.
- **`UserRepository.GetByEmail`** — case-insensitive lookup backed by the
  new `idx_users_lower_email` functional index.

### Three-flag auth split

The legacy single flag `MIGRATOR_PLATFORM_ENABLED` is split into three
independent gates so deployments can enable password auth without
requiring an external NATS server (the multitenant constraint):

- **`AUTH_PASSWORD_ENABLED=true`** — register + login routes + platform DB
  Bootstrap. Compatible with `NATS_MODE=embedded` (zero-config dev).
- **`AUTH_OAUTH_ENABLED=true`** — Google/GitHub OAuth routes + provider
  config. Requires `PLATFORM_BASE_URL` + `OAUTH_SESSION_SECRET`.
- **`MULTITENANT_ENABLED=true`** — per-user tenant DB provisioning via
  JetStream. Still requires `NATS_MODE=external` (operator-JWT account
  isolation).

`MIGRATOR_PLATFORM_ENABLED=true` remains as a legacy alias meaning
"`AUTH_OAUTH_ENABLED=true MULTITENANT_ENABLED=true`" — existing
deployments keep working unchanged.

**Footgun to know about**: `AUTH_PASSWORD_ENABLED` and `AUTH_ENABLED`
(JWT middleware on `/api/v1/*`) are orthogonal. A deployment with only
`AUTH_PASSWORD_ENABLED=true` exposes register + login but leaves
`/api/v1/*` unprotected. Set both flags for production.

## [0.7.0] - 2026-05-09

### Single-Binary DX (v0.7-DX track)

- **`duragraph dev`** — zero-config command that brings up the engine + embedded
  Postgres + embedded NATS + dashboard in one process. No Docker required.
- **`duragraph serve`** — production server (was previously `cmd/server`).
- **`duragraph init <project> [--template hello-world|chatbot|rag|tool-use]`** —
  scaffold a new duragraph project from an embedded template.
- **`duragraph runs {tail|get|trigger}`** — CLI client for the runs API.
- **`duragraph events tail [--aggregate ... --id ...]`** — live-tail the event
  sourcing trail via NATS.
- **Watch mode** — `duragraph dev --watch ./agents` watches a directory for
  Python `@Graph` files and supervises one worker subprocess per file with
  exponential-backoff restart on crashes and SIGTERM-then-SIGKILL on file change.
- **Embedded Postgres + NATS** with high-port defaults (`:15435` and `:14222`)
  to avoid collision with system services on first run. Override via
  `DB_EMBEDDED_PORT` / `NATS_EMBEDDED_PORT` env vars.
- **Studio bundling** — opt-in via `--studio` flag. Studio is embedded into the
  binary at build time alongside the dashboard.

### Multi-tenant Platform (Wave 1)

- **OAuth login flow** (Google + GitHub via goth) at `/api/auth/{provider}/{login,callback}`
- **JWT session middleware** + tenant routing + admin gating
- **Per-tenant Postgres database** (`tenant_<uuid>`) inside shared `prod-postgres`
- **pgxpool-per-tenant** with lazy creation + idle eviction
- **Admin commands** — `ApproveUser` / `RejectUser` / `SuspendUser` / `ResumeUser` /
  `RetryTenantMigration`
- **Admin HTTP handlers** at `/api/admin/{users,tenants,metrics}/*` with Mimir
  PromQL backend for cross-tenant observability
- **Tenant provisioner** (NATS subscriber) that runs CREATE DATABASE + migrations
  on tenant.provisioning events
- **Per-tenant Prometheus labels** on runs/assistants/threads/llm-tokens metrics
- **Capability-aware admin gating** in the dashboard — `/admin/*` routes hidden
  in non-platform deployments
- Gated behind `MIGRATOR_PLATFORM_ENABLED=true` (default false)

### Monorepo migration

The following sibling repos were merged into this monorepo with full git history:

- `Duragraph/duragraph-examples` → `examples/`
- `Duragraph/duragraph-docs` → `docs/`
- `Duragraph/duragraph-python` → `python/` (PyPI dist renamed to `duragraph`)
- `Duragraph/duragraph-go` → `go-sdk/` (module path → `github.com/duragraph/duragraph/go-sdk`)
- `Duragraph/duragraph-studio` → `studio/`

The 5 source repos are now archived (don't delete — history preserved server-side).

### Demo / control-plane fixes

- `task_assignments.Claim` SQL/scan column-count mismatch fixed (workers can now claim tasks)
- `RunRepository.FindByThreadID` no longer fabricates run IDs
- `WorkerHandler.ReceiveEvent` now publishes all 7 event types (was 2) — HITL +
  per-node SSE streaming work end-to-end
- `StreamingBridge` subscribes to `run.RunStarted/Completed/Failed/RequiresAction`
  events that were previously black-holed

### Operator-facing changes

- **CHANGELOG.md** + **RUNBOOK.md** updated for the v0.7 + monorepo state.
- **Release pipeline** (`.github/workflows/release.yml`, `Dockerfile.server`) now
  builds dashboard + studio dists before embedding into the released binary.

## [0.2.0] - 2026-04-13

### Added

- Full LangGraph Cloud API parity — assistants, threads, runs, streaming endpoints
- Event sourcing and CQRS architecture with PostgreSQL, NATS, and Redis
- Worker registration, heartbeat, and task assignment protocol
- MCP server with Streamable HTTP transport
- Crons API for scheduled run execution
- Store API for namespaced key-value storage
- Prometheus metrics and OpenTelemetry tracing
- Rate limiting middleware with configurable env vars
- Horizontal scaling safety for multi-instance deployment
- SSE streaming reliability with per-run NATS topics
- Comprehensive test suite (~54% coverage) across all layers
- Integration tests for PostgreSQL, NATS, and Redis
- GoReleaser pipeline with ko, Cosign signing, SBOM generation
- GitHub Actions CI/CD (tests, conformance, contracts, CodeQL)

### Fixed

- Canonical Apache 2.0 license
- Panic on short model names in LLM provider routing

[0.8.1]: https://github.com/Duragraph/duragraph/releases/tag/v0.8.1
[0.8.0]: https://github.com/Duragraph/duragraph/releases/tag/v0.8.0
[0.7.0]: https://github.com/Duragraph/duragraph/releases/tag/v0.7.0
[0.2.0]: https://github.com/Duragraph/duragraph/releases/tag/v0.2.0
