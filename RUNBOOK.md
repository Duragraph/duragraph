# DuraGraph Runbook

Complete guide for running and managing the DuraGraph application.

> **Multi-tenant deployment?** This runbook covers single-tenant operation.
> For Platform Mode (multi-tenant SaaS with OAuth, admin UI, per-tenant
> databases), see the
> [Platform Mode section in the deployment docs](https://duragraph.ai/docs/ops/deployment/#platform-mode-multi-tenant)
> — covers `MIGRATOR_PLATFORM_ENABLED`, OAuth provider setup, and the
> first-user bootstrap flow.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Repository Layout](#repository-layout)
3. [Quick Start](#quick-start)
4. [Service Management](#service-management)
5. [Accessing Services](#accessing-services)
6. [Development Workflow](#development-workflow)
7. [Troubleshooting](#troubleshooting)
8. [Database Management](#database-management)

---

## Prerequisites

Before running DuraGraph, ensure you have the following installed:

- **Docker & Docker Compose** (for containerized deployment)
- **Task** (task runner) - [Installation Guide](https://taskfile.dev/installation/)
  ```bash
  # macOS
  brew install go-task/tap/go-task

  # Linux
  sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin
  ```

### Optional (for local development):
- **Go 1.22+** - [Download](https://golang.org/dl/)
- **Node.js 22+** - [Download](https://nodejs.org/)
- **pnpm** - `npm install -g pnpm`
- **uv** (for Python SDK + `--watch` worker supervision) - [Installation](https://docs.astral.sh/uv/)

---

## Repository Layout

The repository is a monorepo. Top-level subdirectories:

| Path | What |
|---|---|
| `cmd/duragraph/` | Cobra CLI entry point (`duragraph dev / serve / init / runs / events / migrate`) |
| `internal/` | Engine source (DDD: `domain/`, `application/`, `infrastructure/`) |
| `dashboard/` | Operator/admin React UI (embedded into the binary, served at `/`) |
| `studio/` | Developer/end-user React UI (embedded, opt-in via `duragraph dev --studio`, served at `/studio/`) |
| `examples/` | Python + Go runnable examples (`examples/python/01-hello-world/`, `examples/go/01-hello-world/`) |
| `docs/` | Astro/Starlight docs site (https://duragraph.ai/docs) |
| `python/` | Python SDK (PyPI: `duragraph`) |
| `go-sdk/` | Go SDK (`github.com/duragraph/duragraph/go-sdk`) |
| `deploy/` | Dockerfiles + compose for production |

Each subdirectory has its own dev story; the most common entry points:

```bash
pnpm -C dashboard dev          # dashboard hot-reload (Vite, :3303)
pnpm -C dashboard build        # rebuild static assets so the next go build embeds them
pnpm -C studio dev             # studio hot-reload (Vite, :3300)
pnpm -C studio build           # rebuild studio assets
pnpm -C docs dev               # Astro docs site (:4321)

(cd python && uv sync && uv run pytest)
(cd go-sdk && go test ./...)
```

Per-subproject conventions are in each subdirectory's `CLAUDE.md` / `README.md`. See also the per-project guides under [Memory Files in `CLAUDE.md`](./CLAUDE.md).

---

## Quick Start

Two paths get you a running stack. Pick **Option A** if you have Go locally and want the simplest "first 5 minutes" experience. Pick **Option B** if you want everything in containers (no Go toolchain on your host).

### Option A: zero-config (`duragraph dev`) — recommended

```bash
go run ./cmd/duragraph dev
# or, after `go build -o duragraph ./cmd/duragraph`:
./duragraph dev
```

That single command brings up:

- **Embedded PostgreSQL** on `:15435` (data dir `./data/pg`)
- **Embedded NATS JetStream** on `:14222` (data dir `./data/nats`)
- **Engine + dashboard** on `:8081`
- Schema migrations applied automatically on first start

Visit:

- http://localhost:8081/ — embedded dashboard
- http://localhost:8081/api/v1/ — REST API
- http://localhost:8081/health — health check

Add `--studio` to mount the developer Studio UI at `/studio/`:

```bash
go run ./cmd/duragraph dev --studio
```

Add `--watch ./agents` (default) to supervise Python `@Graph` workers under that directory; pass `--watch ""` to disable.

Stop with `Ctrl+C`. The embedded data dir (`./data`) persists between runs; delete it for a clean slate.

### Option B: Docker Compose (`task up`)

```bash
task up
```

This builds and starts the engine, PostgreSQL, NATS, and dashboard containers via `docker-compose.yml`. Use this when you don't have a Go toolchain locally, or when you want to mirror the production container layout. Endpoints:

```
API + Dashboard - http://localhost:8081
NATS Monitoring - http://localhost:8223
PostgreSQL      - localhost:5433  (host port; container exposes 5432)
```

Then:

```bash
task health   # health check
task ps       # show running containers
```

---

## Service Management

### Start Services

```bash
# Start all services
task up

# Start full development environment
task full
```

### Stop Services

```bash
# Stop all services
task down

# Stop and remove volumes (clean state)
docker compose down -v
```

### Restart Services

```bash
task restart
```

### View Logs

```bash
# All services
task logs

# Specific service
task logs:server
task logs:nats

# Follow logs in real-time
docker compose logs -f
```

---

## Accessing Services

Once services are running, access them at:

| Service | URL | Description |
|---------|-----|-------------|
| **API Server** | http://localhost:8081 | REST API endpoint |
| **Dashboard** | http://localhost:5173 | Web-based workflow visualization |
| **NATS Monitoring** | http://localhost:8222 | NATS JetStream monitoring UI |
| **PostgreSQL** | localhost:5432 | Database (credentials below) |

### Database Credentials

```
Host:     localhost
Port:     5432
Database: appdb
User:     appuser
Password: apppass
```

### API Health Check

```bash
curl http://localhost:8081/health
```

**Expected response:**
```json
{"status":"healthy","version":"2.0.0-ddd"}
```

---

## Development Workflow

There are three modes, ordered by complexity. Pick the one that matches what you're iterating on.

### Mode 1 — zero-config dev (`duragraph dev`)

The v0.7 default. One command brings up the full stack:

```bash
go run ./cmd/duragraph dev
# optional flags:
#   --port 8081           HTTP port for engine + dashboard (default 8081)
#   --data-dir ./data     where embedded postgres + NATS persist (default ./data)
#   --watch ./agents      directory watched for Python @Graph files (default ./agents; "" disables)
#   --studio              also mount the Studio UI at /studio/
```

What you get:

- Embedded PostgreSQL on `:15435` (data: `${data-dir}/pg`)
- Embedded NATS JetStream on `:14222` (data: `${data-dir}/nats`)
- Engine + embedded dashboard on `http://localhost:8081/`
- Optional embedded Studio at `http://localhost:8081/studio/` (with `--studio`)
- Migrations applied automatically; no Docker, no `task up`, no manual `db:migrate`
- File-watcher that spawns a Python worker via `uv run` for each `@Graph`-decorated file under `--watch`

**Use this for:** backend iteration on Go code (when you don't need hot reload), demos, the "first 5 minutes" experience, anything Python-worker-focused.

**Don't expect UI hot reload here.** The dashboard and Studio served at `:8081` are the embedded build artifacts (`dashboard/dist`, `studio/dist`). If you change a `.tsx` file you'll need to `pnpm -C dashboard build` (or `pnpm -C studio build`) and restart the binary, or switch to Mode 2.

### Mode 2 — UI hot reload (two- or three-process)

Use when iterating on **dashboard or Studio source**. The engine still runs in one terminal; Vite dev servers run alongside and proxy `/api/*` calls back to the engine.

```
Browser visits http://localhost:3303 (dashboard) or http://localhost:3300 (studio)
       |
       |   +------------------------------------+
       +-->| Vite dev server (HMR)              |  Terminal 2 / 3
           |  - serves /, /assets/*, /@vite/*   |  pnpm -C dashboard dev
           |  - proxies /api/* to engine        |  pnpm -C studio dev
           +----------------+-------------------+
                            |  proxy /api/*
                            v
           +------------------------------------+
           | duragraph dev :8081                |  Terminal 1
           |  (engine + embedded postgres+NATS) |  go run ./cmd/duragraph dev
           +------------------------------------+
```

```bash
# Terminal 1: engine + embedded data plane
go run ./cmd/duragraph dev

# Terminal 2: dashboard hot-reload (visit http://localhost:3303)
pnpm -C dashboard dev

# Terminal 3 (only if iterating on Studio): studio hot-reload (visit http://localhost:3300)
pnpm -C studio dev
```

**Visit Vite's port (3303 or 3300), NOT 8081.** The UI at `:8081` is the embedded build and won't hot-reload.

**Dashboard proxy gotcha.** The dashboard's Vite config (`dashboard/vite.config.ts`) defaults to proxying `/api/*` to `http://localhost:18081`, not `:8081`. This is to keep dashboard development from clashing with a default `duragraph dev`. Two ways to reconcile:

```bash
# Option (a): point the engine at :18081 to match the dashboard's default
go run ./cmd/duragraph dev --port 18081

# Option (b): leave the engine on :8081 and override the proxy target
VITE_API_PROXY_TARGET=http://localhost:8081 pnpm -C dashboard dev
```

The Studio's Vite config already proxies to `:8081`, so no override is needed for `pnpm -C studio dev`.

**Use this for:** any change in `dashboard/src/**` or `studio/src/**`.

### Mode 3 — engine code hot reload (optional)

If you're iterating on Go code in `internal/` or `cmd/duragraph/` and want auto-restart on save, point any file-watcher at the project. Nothing is preconfigured — the repository does not ship an `.air.toml`. Install [`air`](https://github.com/air-verse/air), [`reflex`](https://github.com/cespare/reflex), or [`entr`](https://github.com/eradman/entr) and watch `./cmd/duragraph` plus `./internal`. For example, with `reflex`:

```bash
reflex -r '\.go$' -s -- go run ./cmd/duragraph dev --studio
```

Combine with Mode 2 (Vite terminals) for Go + React hot iteration.

**Use this for:** simultaneously iterating on engine and UI code.

---

## API Testing

After running `task up`, you can test the API to ensure it's working properly.

### Quick Health Check

```bash
# Check if API is responding
curl http://localhost:8081/health

# Expected response:
# {"status":"healthy","version":"2.0.0-ddd"}
```

### Available API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check endpoint |
| GET | `/metrics` | Prometheus metrics |
| POST | `/api/v1/runs` | Create a new workflow run |
| GET | `/api/v1/runs/:run_id` | Get run details |
| GET | `/api/v1/threads/:thread_id/runs` | List runs for a thread |
| POST | `/api/v1/runs/:run_id/submit_tool_outputs` | Submit tool outputs for a run |
| GET | `/api/v1/stream` | Server-sent events stream |

### Test Examples

#### 1. Health Check
```bash
curl http://localhost:8081/health
```

**Expected Response:**
```json
{
  "status": "healthy",
  "version": "2.0.0-ddd"
}
```

#### 2. Prometheus Metrics
```bash
curl http://localhost:8081/metrics
```

This returns Prometheus-formatted metrics for monitoring.

#### 3. Create a Run

```bash
curl -X POST http://localhost:8081/api/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "thread_id": "123e4567-e89b-12d3-a456-426614174000",
    "assistant_id": "223e4567-e89b-12d3-a456-426614174000",
    "input": {
      "message": "Hello, world!"
    }
  }'
```

**Expected Response:**
```json
{
  "id": "run_abc123...",
  "thread_id": "123e4567-e89b-12d3-a456-426614174000",
  "assistant_id": "223e4567-e89b-12d3-a456-426614174000",
  "status": "queued",
  "created_at": "2025-11-16T20:00:00Z",
  ...
}
```

#### 4. Get Run Status

```bash
# Replace {run_id} with actual run ID from previous response
curl http://localhost:8081/api/v1/runs/{run_id}
```

**Example:**
```bash
curl http://localhost:8081/api/v1/runs/run_abc123
```

**Expected Response:**
```json
{
  "id": "run_abc123",
  "status": "in_progress",
  "thread_id": "123e4567-e89b-12d3-a456-426614174000",
  "assistant_id": "223e4567-e89b-12d3-a456-426614174000",
  "started_at": "2025-11-16T20:00:01Z",
  ...
}
```

#### 5. List Runs for a Thread

```bash
curl http://localhost:8081/api/v1/threads/{thread_id}/runs
```

**Example:**
```bash
curl http://localhost:8081/api/v1/threads/123e4567-e89b-12d3-a456-426614174000/runs
```

#### 6. Stream Events (SSE)

```bash
# Stream real-time events
curl -N http://localhost:8081/api/v1/stream?run_id={run_id}
```

This uses Server-Sent Events (SSE) to receive real-time updates.

### Using HTTPie (Alternative to curl)

If you prefer a more user-friendly HTTP client:

```bash
# Install HTTPie
brew install httpie  # macOS
# or
pip install httpie   # Python

# Health check
http GET http://localhost:8081/health

# Create run (prettier JSON output)
http POST http://localhost:8081/api/v1/runs \
  thread_id="123e4567-e89b-12d3-a456-426614174000" \
  assistant_id="223e4567-e89b-12d3-a456-426614174000" \
  input:='{"message": "Hello, world!"}'
```

### Testing with Postman

1. Import the API endpoints into Postman
2. Create a collection with the endpoints listed above
3. Set base URL: `http://localhost:8081`
4. Test each endpoint

### Testing Script

Create a simple test script to verify all endpoints:

```bash
#!/bin/bash

BASE_URL="http://localhost:8081"

echo "Testing DuraGraph API..."
echo

# Test 1: Health Check
echo "1. Testing health endpoint..."
HEALTH=$(curl -s $BASE_URL/health)
if echo $HEALTH | grep -q "healthy"; then
    echo "   ✅ Health check passed"
else
    echo "   ❌ Health check failed"
    exit 1
fi
echo

# Test 2: Metrics
echo "2. Testing metrics endpoint..."
METRICS=$(curl -s $BASE_URL/metrics)
if echo $METRICS | grep -q "go_goroutines"; then
    echo "   ✅ Metrics endpoint working"
else
    echo "   ❌ Metrics endpoint failed"
fi
echo

# Test 3: Create Run (will fail without valid assistant/thread, but tests endpoint)
echo "3. Testing create run endpoint..."
RUN_RESPONSE=$(curl -s -X POST $BASE_URL/api/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "thread_id": "123e4567-e89b-12d3-a456-426614174000",
    "assistant_id": "223e4567-e89b-12d3-a456-426614174000",
    "input": {"message": "test"}
  }')
echo "   Response: $RUN_RESPONSE"
echo

echo "✅ All basic tests completed!"
```

Save this as `test-api.sh`, make it executable (`chmod +x test-api.sh`), and run it:

```bash
./test-api.sh
```

### Monitoring Logs During Testing

Open a new terminal and watch the logs while testing:

```bash
# Watch all logs
docker logs -f duragraph-server

# Or using task
task logs:server
```

### Common Response Codes

| Code | Meaning | Example |
|------|---------|---------|
| 200 | Success | Request completed successfully |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Invalid input data |
| 404 | Not Found | Resource doesn't exist |
| 500 | Server Error | Internal server error |

### Debugging Failed Requests

If requests fail:

1. **Check server logs:**
   ```bash
   docker logs duragraph-server --tail 50
   ```

2. **Verify services are healthy:**
   ```bash
   task health
   ```

3. **Check database connectivity:**
   ```bash
   docker exec duragraph-server sh -c 'echo "SELECT 1" | psql -h db -U appuser -d appdb'
   ```

4. **Check NATS connectivity:**
   ```bash
   curl http://localhost:8222/healthz
   ```

### Performance Testing

Use Apache Bench or wrk for load testing:

```bash
# Install Apache Bench
brew install httpd  # macOS

# Run 100 requests with 10 concurrent
ab -n 100 -c 10 http://localhost:8081/health

# Or use wrk
wrk -t4 -c100 -d30s http://localhost:8081/health
```

---

## Build & Testing

### Build

```bash
# Build everything
task build

# Build individual components
task build:server      # Builds to bin/server
task build:dashboard
task build:website
```

### Testing

```bash
# Run all tests
task test

# Run specific test suites
task test:unit
task test:integration
task test:conformance
task test:dashboard
```

### Code Quality

```bash
# Lint all code
task lint

# Format all code
task format
```

---

## Database Management

### Connect to Database

```bash
task db:psql
```

This opens a PostgreSQL shell connected to the database.

### Run Migrations

Migrations run automatically on first startup, but you can run them manually:

```bash
task db:migrate
```

### Reset Database

**⚠️ Warning: This deletes all data!**

```bash
task db:reset
```

This will:
1. Stop the database
2. Delete all data volumes
3. Restart the database
4. Run migrations

### Database Operations

```bash
# Start database only
task db:start

# Stop database
task db:stop
```

---

## NATS Operations

### Start/Stop NATS

```bash
task nats:start
task nats:stop
```

### Open Monitoring UI

```bash
task nats:monitor
```

This opens http://localhost:8222 in your browser.

---

## Docker Operations

### Build Docker Images

```bash
# Build all images
task docker:build

# Build individual images
task docker:build:server
task docker:build:dashboard
```

### Clean Up

```bash
# Clean build artifacts
task clean

# Clean everything including Docker volumes
task clean:all
```

---

## Troubleshooting

### Port Conflicts

If you encounter port conflicts:

**Port 8081 already in use:**
```bash
# Find what's using the port
lsof -i :8081

# Kill the process
kill -9 <PID>
```

**Modify ports in docker-compose.yml** if needed.

### Container Health Issues

```bash
# Check container status
docker compose ps

# View specific container logs
docker logs duragraph-server
docker logs duragraph-postgres
docker logs duragraph-nats
docker logs duragraph-dashboard

# Restart unhealthy container
docker compose restart <service-name>
```

### Database Connection Issues

1. **Check if database is healthy:**
   ```bash
   docker compose ps
   ```

2. **Check database logs:**
   ```bash
   docker logs duragraph-postgres
   ```

3. **Test connection manually:**
   ```bash
   docker exec -it duragraph-postgres psql -U appuser -d appdb -c "SELECT 1;"
   ```

### NATS Connection Issues

1. **Verify NATS is healthy:**
   ```bash
   curl http://localhost:8222/healthz
   ```

2. **Check NATS logs:**
   ```bash
   docker logs duragraph-nats
   ```

### Build Failures

**Dashboard build fails:**
```bash
# Rebuild with no cache
docker compose build --no-cache dashboard
```

**Server build fails:**
```bash
# Clean Go cache and rebuild
task clean
task build:server
```

### Complete Reset

If everything is broken, perform a complete reset:

```bash
# Stop and remove everything
docker compose down -v

# Clean build artifacts
task clean

# Rebuild and restart
task up
```

---

## Common Tasks Reference

| Command | Description |
|---------|-------------|
| `task --list` | Show all available tasks |
| `task up` | Start all services |
| `task down` | Stop all services |
| `task restart` | Restart all services |
| `task health` | Check service health |
| `task logs` | View all logs |
| `task ps` | Show running containers |
| `task dev` | Run server in dev mode |
| `task dashboard:dev` | Run dashboard in dev mode |
| `task test` | Run all tests |
| `task build` | Build all components |
| `task clean` | Clean build artifacts |
| `task db:psql` | Connect to database |
| `task db:reset` | Reset database |

---

## Environment Variables

Key environment variables used by the application:

```bash
# Server
PORT=8080                          # Internal server port
HOST=0.0.0.0                       # Server bind address

# Database
DB_HOST=db                         # Database host
DB_PORT=5432                       # Database port
DB_USER=appuser                    # Database user
DB_PASSWORD=apppass                # Database password
DB_NAME=appdb                      # Database name
DB_SSLMODE=disable                 # SSL mode

# NATS
NATS_URL=nats://nats:4222         # NATS connection URL
```

---

## Production Deployment Notes

For production deployment:

1. **Change default passwords** in docker-compose.yml
2. **Enable SSL/TLS** for PostgreSQL (set `DB_SSLMODE=require`)
3. **Use environment-specific configs** for different environments
4. **Set up proper logging and monitoring**
5. **Configure backup strategies** for PostgreSQL and NATS data
6. **Use secrets management** instead of plain environment variables

---

## Getting Help

- **View all available tasks:** `task --list`
- **Task help:** `task <task-name> --help`
- **Docker Compose help:** `docker compose --help`
- **Project Issues:** [GitHub Issues](https://github.com/Duragraph/duragraph/issues)
- **Documentation:** See [docs/](docs/) directory

---

## Version Information

- **DuraGraph:** 2.0.0-ddd
- **PostgreSQL:** 15
- **NATS:** 2.10-alpine
- **Node.js (Dashboard):** 22-alpine
- **Go (Server):** 1.23-alpine
