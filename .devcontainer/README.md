# DuraGraph Development Container

A complete development environment for hacking on DuraGraph. Boots Postgres + NATS as sidecar services, pre-installs Go / Node / Python toolchains, and wires up the dashboard + docs build paths.

## What's included

### Languages and runtimes

- **Go 1.25** — engine and Go SDK
- **Node.js 22** (via fnm) — dashboard (React + Vite) and docs (Astro Starlight)
- **Python 3.12** (via uv) — Python SDK and conformance tests

### Package managers

- **pnpm** — Node.js package manager (dashboard, docs)
- **uv** — Python package manager (never use `pip` directly)
- **go modules** — Go dependency management

### Development tools

- **Task** — task runner (`Taskfile.yml` at the repo root)
- **GitHub CLI (`gh`)** — branch / PR / release ops
- **Docker-in-Docker** — build and run containers from inside the devcontainer
- **PostgreSQL client** — `psql` against the sidecar Postgres
- **Git** — with sensible defaults (rebase pulls, prune on fetch, `main` as default branch)

### Go tools

- `gopls` — language server
- `dlv` — debugger
- `golangci-lint` — linter
- `goimports` — import formatter

### Python tools

- `pre-commit` — runs the hooks declared in `.pre-commit-config.yaml`
- `ruff` — linter + formatter
- `pytest` — test runner

### Shell

- `zsh` (default), `bash` available

## Services

Two sidecar services come up via Docker Compose on the `duragraph-network`:

- **PostgreSQL 15** — `db:5432` from inside the container, `localhost:5432` from your host
- **NATS JetStream** — `nats:4222` from inside, `localhost:4222` from host

```bash
# From inside the devcontainer
postgresql://appuser:apppass@db:5432/appdb
nats://nats:4222

# From your host
postgresql://appuser:apppass@localhost:5432/appdb
nats://localhost:4222
```

These are intended for the long-running dev workflow against external services. If you only need a one-shot self-contained run, `duragraph dev` boots **embedded** Postgres + NATS instead and you can ignore the sidecars entirely.

## Port forwarding

| Port | Service                            | Auto-forward |
| ---- | ---------------------------------- | ------------ |
| 8081 | DuraGraph API + embedded dashboard | Notify       |
| 3303 | Dashboard dev server (Vite)        | Notify       |
| 4321 | Docs dev server (Astro)            | Notify       |
| 5432 | PostgreSQL                         | Silent       |
| 4222 | NATS                               | Silent       |
| 8222 | NATS monitoring UI                 | Silent       |

## VS Code extensions

### Backend

- `golang.go` — Go language support
- `ms-python.python` + `ms-python.vscode-pylance` — Python
- `ms-azuretools.vscode-docker` — Docker

### Frontend

- `dbaeumer.vscode-eslint` — ESLint
- `esbenp.prettier-vscode` — Prettier
- `bradlc.vscode-tailwindcss` — Tailwind CSS

### Productivity

- `task.vscode-task` — Task runner integration
- `anthropic.claude-code` — Claude Code
- `github.copilot` + `github.copilot-chat` — Copilot
- `eamodio.gitlens` — Git history / blame

### Code quality

- `usernamehw.errorlens` — inline diagnostics
- `gruntfuggly.todo-tree` — TODO scanner
- `streetsidesoftware.code-spell-checker` — spell checker

## First-time setup

`post-create.sh` runs automatically on container build and:

1. Installs Go toolchain (`gopls`, `dlv`, `golangci-lint`, `goimports`) and `go mod download`s the engine
2. `pnpm install`s the dashboard and docs
3. Installs Playwright browsers if the dashboard pulls in `@playwright/test`
4. Installs the pre-commit hooks (`pre-commit install` + `--hook-type commit-msg`)
5. Sets git defaults (`pull.rebase=true`, `fetch.prune=true`, `init.defaultBranch=main`)
6. Optionally configures git user, GPG signing, and `gh` auth from env vars (all optional — see Configuration below)

## Common commands

```bash
# Run the engine in dev mode (embedded Postgres + NATS, dashboard served at :8081)
duragraph dev

# Or against the sidecar Postgres + NATS:
task dev

# Dashboard
task install:dashboard      # one-time pnpm install
task dashboard:dev          # vite dev server on :3303
task build:dashboard        # production build into dashboard/dist/

# Engine + types
task build:duragraph        # build the engine binary to bin/duragraph
task gen:types              # regenerate dashboard/src/types/generated.ts from Go DTOs (tygo)

# Tests
task test                   # full test suite
task test:go                # Go unit tests
task test:integration       # integration tests (real Postgres + NATS)
task test:conformance       # API conformance suite
task test:dashboard         # dashboard tests
task test:e2e               # end-to-end suite
task test:soak              # load / soak tests

# Lint + format
task lint                   # lint all (Go + dashboard)
task lint:go
task lint:dashboard
task format                 # format all
task format:go
task format:dashboard

# Docker
task docker:build           # build all images
task docker:build:api       # just the engine image

# Misc
task health                 # health-check the API
task clean                  # clean build artifacts
task --list                 # full target list
```

Docs site has no Taskfile wrapper — run it directly from `docs/`:

```bash
cd docs
pnpm dev          # astro dev on :4321
pnpm build        # static build into docs/dist/
pnpm preview      # serve the built site locally
```

## Configuration

### Environment variables (auto-set)

- `DOCKER_BUILDKIT=1` — BuildKit for Docker builds
- `COMPOSE_DOCKER_CLI_BUILD=1` — BuildKit with Compose
- `PATH` — includes Go, Node (fnm), Python (uv), and tool binaries

### Git (all optional)

Defaults to your host git config. Override per-devcontainer via env vars:

- `GIT_USER_NAME` — git user name
- `GIT_USER_EMAIL` — git user email
- `GPG_KEY_ID` + `GPG_PRIVATE_KEY` (base64) — GPG key for signed commits (maintainers only)
- `ENABLE_GPG_SIGNING=true` — auto-sign every commit
- `GH_PAT` — `gh` CLI auth token

GPG signing is **not** required for contributing — these are entirely optional.

### Default git settings applied to all users

- Default branch: `main`
- Pull strategy: `rebase`
- Auto-prune on fetch

### Shell

- Default: `zsh`
- `fnm` (Fast Node Manager) auto-loaded in `.zshrc` and `.bashrc`
- `uv` available on `PATH` via `~/.local/bin`

## Workspace mounts

- `~/.claude` (host) → `/home/vscode/.claude` (container) — preserves Claude Code configuration across rebuilds.

## Rebuilding the container

```bash
# From VS Code: Command Palette → "Dev Containers: Rebuild Container"

# Or manually:
docker compose -f .devcontainer/docker-compose.yml down
docker compose -f .devcontainer/docker-compose.yml up -d --build
```

## Troubleshooting

**Go modules not resolving**

```bash
go mod download && go mod tidy
```

**Node / pnpm not found**

```bash
eval "$(fnm env)"
fnm install 22
```

**Python not found**

```bash
uv python install 3.12
```

**Docker socket issues** — Docker-in-Docker is enabled; the socket should be at `/var/run/docker.sock`. If it's missing, rebuild the container.

**Port already in use**

```bash
sudo lsof -i :8081           # find the process
duragraph dev --port 9000    # or override the engine port
```

## Resources

- [VS Code Dev Containers](https://code.visualstudio.com/docs/devcontainers/containers)
- [Task](https://taskfile.dev/) — see `Taskfile.yml` at the repo root for the canonical target list
- [fnm](https://github.com/Schniz/fnm) — Node version manager
- [uv](https://github.com/astral-sh/uv) — Python package manager
