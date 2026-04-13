---
applyTo: "**/*.go"
---

# Go Code Review Instructions

## Project Overview

DuraGraph is an enterprise-grade, LangGraph Cloud-compatible AI workflow orchestration control plane. It is a single Go module (`github.com/duragraph/duragraph`) built with Go 1.25, Echo v4, PostgreSQL (pgx v5), NATS JetStream, and OpenTelemetry.

The control plane **dispatches tasks to SDK workers** — it does NOT make LLM calls itself. LLM calls happen in Python/Go SDK workers that register with the control plane.

## Architecture — Domain-Driven Design with CQRS + Event Sourcing

```
internal/
├── pkg/                 # Shared utilities (errors, eventbus, uuid) — zero dependencies
├── domain/              # Pure domain logic — NO infrastructure imports allowed
│   ├── run/             # Run aggregate (workflow execution state machine)
│   ├── worker/          # Worker aggregate (registration, heartbeat, leases)
│   ├── workflow/        # Workflow aggregates (Assistant, Thread, Graph)
│   ├── execution/       # Execution state machine, node executors
│   ├── checkpoint/      # State snapshots for thread resumption
│   ├── humanloop/       # Human-in-the-loop interrupts
│   └── messaging/       # Domain messaging interfaces
├── application/         # Use cases — orchestrates domain + infrastructure
│   ├── command/         # Write operations (CreateRun, CreateAssistant, etc.)
│   ├── query/           # Read operations (GetRun, ListAssistants, etc.)
│   └── service/         # Domain services (RunService, WorkerService, CronScheduler)
└── infrastructure/      # External concerns — implements domain interfaces
    ├── http/            # REST API (handlers, middleware, DTOs)
    ├── persistence/     # PostgreSQL repositories, event store, outbox
    ├── messaging/       # NATS publisher/subscriber, outbox relay
    ├── graph/           # Graph execution engine
    ├── execution/       # LLM/tool executor wiring
    ├── llm/             # LLM provider clients (OpenAI, Anthropic)
    ├── tools/           # Tool registry
    ├── auth/            # JWT authentication
    ├── cache/           # Redis caching
    ├── mcp/             # Model Context Protocol server
    ├── streaming/       # SSE streaming bridge
    ├── monitoring/      # Prometheus metrics
    └── tracing/         # OpenTelemetry tracing
```

## Strict Layer Rules

1. **`domain/` packages must NEVER import from `application/` or `infrastructure/`**. Domain is pure business logic with no external dependencies.
2. **`application/` may import `domain/` but never `infrastructure/`**. Application layer depends on domain interfaces, not implementations.
3. **`infrastructure/` implements interfaces defined in `domain/`** (Repository pattern).
4. **`pkg/` is shared utilities** — importable by any layer, but must not import from `internal/`.

## Domain Aggregate Pattern

All aggregates follow this pattern:
- Unexported fields with getter methods
- Factory function (`NewXxx`) that validates input and records a domain event
- `Reconstruct` / `Reconstitute` function for rehydration from persistence (no events emitted)
- `Events() []eventbus.Event` returns uncommitted events
- `ClearEvents()` resets after persistence
- State changes MUST record domain events — events are the source of truth

```go
type Run struct {
    id     string
    status RunStatus
    events []eventbus.Event
}

func NewRun(...) (*Run, error) {
    // validate, create, record RunCreated event
}

func (r *Run) Events() []eventbus.Event { return r.events }
```

## Run State Machine

```
queued → in_progress → completed
       ↓              ↓
       ↓              failed
       ↓              ↓
       ↓              cancelled
       ↓
       requires_action → (resume) → in_progress
```

Only valid transitions are allowed. Invalid state transitions must return `errors.InvalidState()`.

## Error Handling

Use the centralized error package at `internal/pkg/errors`:

```go
errors.NotFound("assistant", id)
errors.AlreadyExists("thread", id)
errors.InvalidInput("field", "reason")
errors.InvalidState("current", "attempted")
errors.Internal("message", wrappedErr)
```

- All domain errors wrap sentinel errors (`ErrNotFound`, `ErrInvalidInput`, etc.) for `errors.Is()` checking
- Never use raw `fmt.Errorf` in domain code — use `DomainError` types
- Infrastructure code may use standard Go errors but should wrap with context

## Event Sourcing Rules

- **Events are immutable** — never modify the `events` table
- **Event store + outbox must be atomic** — always within the same transaction
- **Events are replayed in order** to reconstruct aggregate state
- **Aggregates must not call repositories** — keep domain pure

## Code Style

- `gofmt` formatting is enforced (pre-commit hook)
- `goimports` for import ordering (stdlib, external, internal)
- No `golangci-lint` yet — use `go vet` for static analysis
- Conventional commits required: `feat:`, `fix:`, `test:`, `refactor:`, `docs:`, `chore:`
- Package-level comments explain purpose; exported types have descriptive comments
- Test files: `*_test.go` in the same package (white-box testing preferred for domain)
- Integration tests use build tag `//go:build integration`
- E2E tests use build tag `//go:build e2e`

## Testing Expectations

When reviewing test code:
- Unit tests must have `-short` flag support and no external dependencies
- Use `testing.T` standard library — `testify` is available but table-driven tests with stdlib are preferred for new code
- Test helpers should call `t.Helper()`
- Domain tests should verify both behavior AND event emission
- Each aggregate method that changes state should emit a corresponding domain event
- Verify error types with `errors.Is()`, not string matching

## Key Entry Points

- **Server entry**: `cmd/server/main.go`
- **Config**: `cmd/server/config/config.go` (reads env vars with `os.Getenv`, no .env loading)
- **HTTP routes**: registered in `internal/infrastructure/http/handlers/`
- **Database migrations**: `deploy/sql/001_init.sql` through `deploy/sql/012_crons.sql`
- **OpenAPI spec**: `schemas/openapi/duragraph.yaml`
- **Docker Compose**: `docker-compose.yml` (dev), `deploy/compose/docker-compose.test.yml` (CI)
