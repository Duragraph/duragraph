# Smoke the Python worker against the shipped v2 binary

Run from the repository root. This uses **isolated, disposable** Postgres/NATS
containers and a binary built from `./cmd/duragraph`; it does not exercise an
in-process test server or the old worker polling endpoints. Docker and Go are
required. Choose a free HTTP port if 39081 is occupied.

```sh
go build -o /tmp/opencode/duragraph-python-v2-smoke ./cmd/duragraph
docker run --rm -d --name dg-python-v2-smoke-pg -e POSTGRES_USER=smoke -e POSTGRES_PASSWORD=smoke -e POSTGRES_DB=smoke -P postgres:15
docker run --rm -d --name dg-python-v2-smoke-nats -P nats:2.10-alpine -js
docker port dg-python-v2-smoke-pg 5432/tcp
docker port dg-python-v2-smoke-nats 4222/tcp
```

Use the mapped ports printed by Docker below (example: `34177`, `34178`).
Wait until `docker exec dg-python-v2-smoke-pg pg_isready -U smoke -d smoke`
reports ready. In a separate terminal, start the **opt-in** v2 server:

```sh
DB_HOST=127.0.0.1 DB_PORT=34177 DB_USER=smoke DB_PASSWORD=smoke DB_NAME=smoke DB_SSLMODE=disable \
NATS_URL=nats://127.0.0.1:34178 HOST=127.0.0.1 PORT=39081 \
/tmp/opencode/duragraph-python-v2-smoke serve --control-plane=v2
```

From `python/`, run the opt-in integration tests (normal CI skips without
`DURAGRAPH_V2_URL`):

```sh
DURAGRAPH_V2_URL=http://127.0.0.1:39081 uv run --frozen --extra dev pytest -q -s tests/test_worker_v2_integration.py
```

The suite registers a graph and worker via the SDK, creates a threaded run,
claims a lease, executes a node, writes events/checkpoint, and asserts the
public run status and latest checkpoint. It also requeues a claimed run to a
second worker and verifies the old epoch is fenced; a separate run exercises
`run.failed`. Inspect persisted output (the public `Run` response does not
include its output) with the run UUID printed by the passing smoke:

```sh
docker exec dg-python-v2-smoke-pg psql -U smoke -d smoke -c \
  "SELECT r.id, r.status, r.output, r.lease_epoch, s.id AS checkpoint_id, s.state FROM runs r JOIN snapshots s ON s.aggregate_id = r.id WHERE r.id = 'RUN_UUID_FROM_TEST'"
```

Expected: `completed`, output `{"name":"v2","answer":"hello v2"}`,
lease epoch >= 1 and checkpoint state with that output under `channels`.
Stop the server (Ctrl+C), then stop **only these two** containers with
`docker stop dg-python-v2-smoke-pg dg-python-v2-smoke-nats`.

**Known adjacent issue:** on the current v2 server the NATS outbox relay
reports `no response from stream` for subjects such as
`duragraph.events.assistant.created` and `duragraph.events.thread.created`.
The HTTP worker claim/execute/checkpoint smoke succeeds, but this test does
not prove relay delivery or SSE. The Python SDK does not configure streams.
