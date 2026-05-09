# Changelog

All notable changes to DuraGraph will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.2.0]: https://github.com/Duragraph/duragraph/releases/tag/v0.2.0
