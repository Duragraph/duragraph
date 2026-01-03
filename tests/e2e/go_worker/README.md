# Go Mock Worker

A Go implementation of the DuraGraph worker protocol for E2E testing.

## Quick Start

```bash
# Build
go build -o go_worker .

# Run (connects to local control plane)
CONTROL_PLANE_URL=http://localhost:8082 ./go_worker
```

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROL_PLANE_URL` | `http://localhost:8081` | Control plane API URL |
| `WORKER_ID` | auto-generated | Worker identifier |
| `WORKER_NAME` | `go-mock-worker` | Worker display name |
| `MOCK_WORKER_GRAPH` | `simple_echo` | Default graph to use |
| `MOCK_WORKER_DELAY_MS` | `100` | Delay between nodes (ms) |
| `MOCK_WORKER_FAIL_AT_NODE` | - | Node ID to fail at |
| `MOCK_WORKER_INTERRUPT_AT_NODE` | - | Node ID to interrupt at |
| `MOCK_WORKER_TOKEN_COUNT` | `100` | Simulated tokens per LLM call |
| `MAX_CONCURRENT_RUNS` | `5` | Maximum concurrent executions |
| `HEARTBEAT_INTERVAL` | `10s` | Heartbeat frequency |
| `POLL_INTERVAL` | `1s` | Task polling frequency |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

## Available Graphs

| Graph ID | Description |
|----------|-------------|
| `simple_echo` | Basic input → LLM → output |
| `multi_step` | Multiple LLM calls in sequence |
| `branching` | Conditional routing based on classification |
| `tool_calling` | Demonstrates tool usage (search, calculator) |
| `human_interrupt` | Requires human approval mid-execution |
| `long_running` | 5 sequential steps with delays |
| `failure` | Simulates execution failures |

## Docker

```bash
# Build image
docker build -t go-mock-worker .

# Run container
docker run -e CONTROL_PLANE_URL=http://host.docker.internal:8081 go-mock-worker
```

## With Docker Compose

```bash
# Run with the go-worker profile
docker compose -f tests/e2e/docker-compose.test.yml --profile go-worker up go-mock-worker
```

## Testing Scenarios

### Test failure handling
```bash
MOCK_WORKER_FAIL_AT_NODE=process ./go_worker
```

### Test human-in-the-loop
```bash
MOCK_WORKER_INTERRUPT_AT_NODE=review ./go_worker
```

### Test slow execution
```bash
MOCK_WORKER_DELAY_MS=1000 ./go_worker
```

## Project Structure

```
go_worker/
├── main.go           # Entry point
├── config/
│   └── config.go     # Environment configuration
├── graphs/
│   └── graphs.go     # Graph definitions
├── executor/
│   └── executor.go   # Graph execution engine
├── worker/
│   └── worker.go     # Worker protocol
├── Dockerfile
├── go.mod
└── README.md
```
