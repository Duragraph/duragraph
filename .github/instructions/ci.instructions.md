---
applyTo: "**"
---

# DuraGraph Control Plane — General Instructions

## Build & Test Commands

All commands run from the repository root.

### Required toolchain

- Go 1.25+
- Python 3.11+ (for pre-commit hooks)
- `goimports` (`go install golang.org/x/tools/cmd/goimports@latest`)
- `pre-commit` (`pip install pre-commit`)

### Build

```bash
go build ./cmd/server/          # Build server binary
go vet ./...                     # Static analysis (run before every commit)
```

### Test

```bash
go test -short ./...             # All unit tests (no external deps)
go test -short -cover ./...      # Unit tests with coverage
go test -tags integration ./...  # Integration tests (needs Postgres, NATS, Redis)
go test -tags e2e ./tests/e2e/   # End-to-end tests (needs running server)
```

Always run `go test -short ./...` before submitting changes. This is what CI runs.

### Pre-commit hooks (enforced in CI)

```bash
pre-commit run --all-files       # Run all hooks locally
```

Hooks that run on every commit:
- `trailing-whitespace`, `end-of-file-fixer`
- `check-yaml`, `check-json`
- `check-added-large-files` (max 1MB)
- `check-merge-conflict`, `detect-private-key`
- `go-fmt`, `go-imports`, `go-mod-tidy`, `go-vet`
- `gitleaks` (secret detection)
- `commitizen` (conventional commit format on commit messages)

### Taskfile (optional)

```bash
task dev          # Start server with hot reload
task up           # Start full Docker Compose environment
task health       # Check server health
task test         # Run all tests
task test:unit    # Unit tests only
task db:migrate   # Run database migrations
```

## CI Checks (must all pass on PRs)

| Check | Workflow | What it validates |
|-------|----------|-------------------|
| **Pre-commit Checks** | `ci.yml` | go-fmt, go-imports, go-mod-tidy, go-vet, trailing whitespace, YAML/JSON, gitleaks, commitizen |
| **Go Tests** | `ci.yml` | `go vet ./...` + `go test ./... -cover -short` |
| **Conformance Tests** | `conformance.yml` | Docker Compose integration test suite against LangGraph Cloud API spec |
| **OpenAPI Lint** | `contracts.yml` | Spectral lint of `schemas/openapi/duragraph.yaml` |
| **IR Validate** | `contracts.yml` | JSON Schema validation of IR examples |
| **CodeQL** | `codeql.yml` | Security analysis (Go, JS/TS, Python) |
| **Issue Automation** | `issue-automation.yml` | Auto-updates issue labels on PR events |

## Commit Messages

Conventional Commits format is enforced by commitizen pre-commit hook:

```
feat: add webhook event definitions
fix: resolve worker reconnection issue
test: add unit tests for execution state machine
refactor: extract LLM provider interface
docs: update API reference
chore: update dependencies
```

Scope is optional: `feat(http): add rate limiting middleware`

## File Structure Reference

```
cmd/server/              # Server entry point and config
internal/
├── pkg/                 # Shared utilities (errors, eventbus, uuid)
├── domain/              # Pure domain logic (aggregates, events, interfaces)
├── application/         # Use cases (command handlers, query handlers, services)
└── infrastructure/      # External concerns (HTTP, DB, NATS, LLM, etc.)
deploy/
├── sql/                 # Database migrations (001-012)
└── compose/             # Docker Compose files for testing
schemas/
└── openapi/             # OpenAPI 3.0 specification
tests/
└── e2e/                 # End-to-end test suite
```

## Important Constraints

- **Never push to `main` directly** — all changes via PR
- **Never modify the `events` table** — events are append-only and immutable
- **Domain packages must not import infrastructure** — strict layer boundaries
- **The control plane does not make LLM calls** — it dispatches to workers
- **`go.sum` changes are expected** when dependencies change — always commit both `go.mod` and `go.sum`
- **No `.env` file loading in code** — server reads raw `os.Getenv()` calls
