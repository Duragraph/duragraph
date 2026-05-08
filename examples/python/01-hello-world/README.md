# Hello World

The simplest DuraGraph example: a worker that registers a two-node graph with the control plane, then executes runs dispatched to it.

## What this demonstrates

- Defining a graph with `@Graph(id=...)` and `@node()` decorators
- Connecting nodes with the `>>` operator
- Registering a worker with the control plane
- Running a graph end-to-end (`queued → in_progress → completed`)
- Inspecting the event-sourced trail in Postgres

## Prerequisites

- DuraGraph control plane reachable (defaults to `http://localhost:8081`; this demo uses `18081`)
- Python 3.11+
- `uv` installed ([installation](https://docs.astral.sh/uv/))

## Run the worker

> **Always use `uv` — never `pip install`, never `python -m venv`, never `source .venv/bin/activate`.**

The Python SDK lives in the sibling repo `duragraph-python/`. We install it as an editable dep via `uv run`:

```bash
DURAGRAPH_URL=http://localhost:18081 PYTHONUNBUFFERED=1 \
  uv run --with-editable /home/qwe/platform/duragraph-org/duragraph-python \
  python main.py
```

The worker:

1. Runs a one-shot local-execution demo (you'll see two `RuntimeWarning`s — those come from a known SDK bug in the local-only path; the worker path that follows is unaffected)
2. Registers itself with the control plane as `hello-world-worker`
3. Begins polling for tasks

---

## Demo cheat sheet

Setup assumes the local-dev stack is up (see `../../docker-compose/local-dev/`). All commands run from this VPS; the demo is reached from a laptop via SSH port forwarding.

### SSH port forwarding (run on your laptop)

```bash
ssh -L 18081:localhost:18081 \
    -L 19090:localhost:19090 \
    -L 5435:localhost:5435 \
    vps-host
```

### Stack state

```
docker ps
  duragraph-dev-app        → API on host 18081, metrics on 19090
  duragraph-dev-postgres   → host 5435 (5432 inside)
  duragraph-dev-nats       → internal only (no host port)
```

### 1. Health check

```bash
curl -s http://localhost:18081/health | jq
```

Expected: `{"status":"healthy","version":"2.0.0-ddd"}`.

### 2. Show no workers registered yet

```bash
curl -s http://localhost:18081/api/v1/workers | jq
```

### 3. Start the Python worker (in pane A)

```bash
cd /home/qwe/platform/duragraph-org/duragraph-examples/python/01-hello-world
DURAGRAPH_URL=http://localhost:18081 PYTHONUNBUFFERED=1 \
  uv run --with-editable /home/qwe/platform/duragraph-org/duragraph-python \
  python main.py
```

Wait for `✓ Registered with worker_id: hello-world-worker`.

### 4. Confirm the worker is registered (pane B)

```bash
curl -s http://localhost:18081/api/v1/workers | jq
```

You should see `status: ready` and `graphs: ["hello_world"]`.

### 5. Create assistant + thread + run

```bash
ASSISTANT_ID=$(curl -s -X POST http://localhost:18081/api/v1/assistants \
  -H "Content-Type: application/json" \
  -d '{"name":"hello-assistant","graph_id":"hello_world","metadata":{"graph_id":"hello_world"}}' \
  | jq -r .assistant_id)

THREAD_ID=$(curl -s -X POST http://localhost:18081/api/v1/threads \
  -H "Content-Type: application/json" -d '{}' | jq -r .thread_id)

RUN_ID=$(curl -s -X POST "http://localhost:18081/api/v1/threads/$THREAD_ID/runs" \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\":\"$ASSISTANT_ID\",\"input\":{\"name\":\"InterviewDemo\"}}" \
  | jq -r .run_id)

echo "RUN $RUN_ID"
```

### 6. Watch the run complete

```bash
curl -s "http://localhost:18081/api/v1/runs/$RUN_ID" | jq
```

Expected output:

```json
{
  "run_id": "...",
  "status": "completed",
  "input":  {"name": "InterviewDemo"},
  "output": {
    "name": "InterviewDemo",
    "greeting": "Hello, InterviewDemo!",
    "farewell": "Goodbye! Thanks for using DuraGraph."
  },
  "started_at": "...",
  "completed_at": "..."
}
```

In pane A you'll see:

```
📥 Received work (HTTP poll): <run_id>
[greet] Hello, InterviewDemo!
[farewell] Goodbye! Thanks for using DuraGraph.
```

### 7. Show the event-sourced trail

```bash
docker exec duragraph-dev-postgres psql -U duragraph -d duragraph \
  -c "SELECT event_type, occurred_at FROM events WHERE aggregate_id='$RUN_ID' ORDER BY id;"
```

Expected: `RunCreated, RunStarted, RunCompleted` with timestamps.

### 8. Show the outbox (events shipped to NATS)

```bash
docker exec duragraph-dev-postgres psql -U duragraph -d duragraph \
  -c "SELECT event_type, published, attempts FROM outbox WHERE aggregate_id='$RUN_ID' ORDER BY id;"
```

All rows should show `published = t`, `attempts = 0`.

---

## Talking points while demoing

- **The worker advertises its graph capability** — the control plane only dispatches `hello_world` runs to workers that declared support for that graph.
- **The Run aggregate enforces a state machine** — `queued → in_progress → completed`. Try cancelling a completed run to show illegal transitions are rejected (`409 Conflict`).
- **Every state change is an event** — the `events` table is append-only; the `outbox` table is what bridges the synchronous DB transaction to the asynchronous NATS bus.
- **User code runs in the Python worker process**, never on the Go control plane. The control plane orchestrates dispatch, leases, and persistence; the worker owns the `@node` method bodies.

## Code walkthrough

### Graph definition

```python
@Graph(id="hello_world")
class HelloWorld:
    @entrypoint
    @node()
    async def greet(self, state: dict) -> dict:
        name = state.get("name", "World")
        state["greeting"] = f"Hello, {name}!"
        return state

    @node()
    async def farewell(self, state: dict) -> dict:
        state["farewell"] = "Goodbye! Thanks for using DuraGraph."
        return state

    greet >> farewell
```

- `@Graph(id=...)` marks a class as a workflow graph and assigns a stable ID.
- `@entrypoint` marks the start node.
- `>>` connects nodes — `greet >> farewell` means "after `greet`, run `farewell`."

### Worker bootstrap

`main.py` calls `agent.serve(control_plane_url)` which under the hood:

1. `POST /api/v1/workers/register` — uploads the graph IR.
2. Starts a heartbeat loop (every 30s).
3. Starts a poll loop — `POST /api/v1/workers/{id}/poll` to claim tasks.
4. Per claimed task: walks the graph, posts `node_started`/`node_completed`/`run_completed` events back to the control plane.

## Configuration

| Environment variable | Default | Description |
| --- | --- | --- |
| `DURAGRAPH_URL` | `http://localhost:8081` | Control plane URL the worker registers against |
| `PYTHONUNBUFFERED` | (unset) | Set to `1` for live stdout in long-running worker |

## Troubleshooting

- **Worker registration fails with 500 a few times then succeeds** — control plane is still warming up (lease monitor's `pg_advisory_lock`, etc.). Give it ~10s after `docker compose up`.
- **Run stays in `in_progress` forever** — check that the worker is registered (`curl /api/v1/workers`) AND that its `graphs` array includes the assistant's `graph_id`.
- **`coroutine ... was never awaited` warnings** at startup — known SDK bug in the local-execution path (`agent.run()` calls async methods synchronously). Doesn't affect the worker; ignore.

## Next steps

- [02-chatbot](../02-chatbot) — Add conversation memory
- [06-tool-use](../06-tool-use) — Call external tools
