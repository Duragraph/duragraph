# E2E Tests

End-to-end tests for DuraGraph, including mock workers and infrastructure tests.

## Quick Start

```bash
# Run E2E tests locally (requires control plane running)
./run-e2e-tests.sh --local

# Run E2E tests in Docker
./run-e2e-tests.sh --docker
```

## Mock Workers

Two worker implementations are available for testing:

| Worker | Language | Location | Use Case |
|--------|----------|----------|----------|
| `mock-worker` | Python | `mock_worker/` | Default, async-native |
| `go-mock-worker` | Go | `go_worker/` | Performance testing |

Both workers implement the same 7 graph patterns and worker protocol.

### Running Workers Locally

**Python Worker:**
```bash
cd mock_worker
uv pip install -r requirements.txt
CONTROL_PLANE_URL=http://localhost:8082 uv run python -m mock_worker.main
```

**Go Worker:**
```bash
cd go_worker
go build -o go_worker .
CONTROL_PLANE_URL=http://localhost:8082 ./go_worker
```

## Docker Compose

### Start Full E2E Environment

```bash
# Start all services (PostgreSQL, NATS, control plane, Python worker)
docker compose -f docker-compose.test.yml up

# Use Go worker instead
docker compose -f docker-compose.test.yml --profile go-worker up
```

### Run Tests

```bash
# Run test runner container
docker compose -f docker-compose.test.yml run test-runner
```

## Test Structure

```
tests/e2e/
├── infrastructure/           # Infrastructure tests
│   └── test_worker_protocol.py   # Worker protocol E2E tests
├── mock_worker/              # Python mock worker
├── go_worker/                # Go mock worker
├── docker-compose.test.yml   # E2E test environment
├── Dockerfile.test           # Test runner container
├── run-e2e-tests.sh          # Test runner script
└── README.md
```

## Available Graphs

All mock workers support these graph patterns:

| Graph | Nodes | Description |
|-------|-------|-------------|
| `simple_echo` | 3 | Basic echo (input → LLM → output) |
| `multi_step` | 5 | Sequential LLM calls |
| `branching` | 7 | Conditional routing |
| `tool_calling` | 6 | Tool usage demo |
| `human_interrupt` | 5 | Human-in-the-loop |
| `long_running` | 7 | Slow execution simulation |
| `failure` | 4 | Error simulation |

## Configuration

Workers are configured via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROL_PLANE_URL` | `http://localhost:8081` | API server URL |
| `MOCK_WORKER_GRAPH` | `simple_echo` | Default graph |
| `MOCK_WORKER_DELAY_MS` | `100` | Node delay (ms) |
| `MOCK_WORKER_FAIL_AT_NODE` | - | Force failure at node |
| `MOCK_WORKER_INTERRUPT_AT_NODE` | - | Force interrupt at node |
| `LOG_LEVEL` | `info` / `INFO` | Logging level |

## Test Scenarios

### Basic Worker Protocol
```bash
# Tests registration, heartbeat, polling, deregistration
uv run pytest infrastructure/test_worker_protocol.py -v
```

### Failure Handling
```bash
MOCK_WORKER_FAIL_AT_NODE=process ./go_worker
```

### Human-in-the-Loop
```bash
MOCK_WORKER_INTERRUPT_AT_NODE=review ./go_worker
```

## CI Integration

The test runner outputs JUnit XML for CI integration:

```bash
docker compose -f docker-compose.test.yml run test-runner
# Results in: /results/e2e-results.xml
```
