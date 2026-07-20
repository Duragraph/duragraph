# Control Plane Rebuild — Structure

Ground-up rebuild of the DuraGraph control plane. **Source of truth: the d2
diagrams in `spec/models/` (d2lang.com).** The old `internal/` tree is being
replaced, not reused. Build order is bottom-up; layers 1–3 are built
independently, then combined.

| Layer | Folder | Built from (d2 source of truth) | What it holds |
|-------|--------|----------------------------------|---------------|
| 1. Schema | `db/migrations/` | `models/d2/postgres.d2` | Consolidated, numbered SQL migrations — every table in postgres.d2 (platform, workflow, run, store, event-sourcing contexts). |
| 2. Outbox TX | `db/outbox/` | `models/d2/relay.d2` + `outbox/transactional-outbox.yml` | The transactional-outbox SQL: write TX (INSERT events → INSERT outbox → pg_notify), drain SELECT, mark-published / mark-failed. Reused by every write endpoint. |
| 3. Endpoints | `endpoints/` (generated) + `gen/` (generator) | `models/d2/endpoint-queries.d2` + `models/d2/api.d2` + `spec/api/duragraph-latest.yaml` | One Go handler per LangGraph endpoint, **generated via Go templates**. endpoint-queries.d2 already lists per-endpoint: SQL steps, outbox flag, NATS subject. OpenAPI supplies request/response types. |
| 4. Assembly | `server/` | `models/system-architecture.d2` | Composition root: wires migrations + outbox + endpoints into one binary. |
| 4. NATS | `nats/` | `models/d2/nats.d2` | Streams + consumers; relay publishes, SSE subscribes. |
| 5. Worker | *(separate — duragraph SDK)* | `models/d2/workers.d2` | NOT in this binary. Worker attaches via the duragraph SDK over NATS. |

Endpoint groups (from endpoint-queries.d2): assistants, threads, runs, crons,
store, workers, auth, platform, admin, system.
