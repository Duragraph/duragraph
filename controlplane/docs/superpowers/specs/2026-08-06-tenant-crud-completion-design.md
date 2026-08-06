# Tenant CRUD completion — assistants/threads/runs remaining reads + writes

**Date:** 2026-08-06
**Branch:** `feat/controlplane-tenant-crud` (off `main`)
**Status:** approved design, pending implementation plan

## Context

The tenant endpoint groups are mostly done. This slice fills the remaining
well-defined read/write stubs so the tenant CRUD surface is complete. It excludes
stubs that depend on subsystems not yet built.

### Scope

**In (8):**
| Endpoint | Method / path | Kind | Approach |
|----------|---------------|------|----------|
| assistants.get_graph | `GET /assistants/{id}/graph` | read | SELECT from `graphs` by assistant → GraphSchema |
| threads.get_state | `GET /threads/{id}/state` | read | latest `snapshots` row for the thread's runs |
| threads.get_checkpoint_state | `GET /threads/{id}/state/{checkpoint_id}` | read | `snapshots` by id, thread-scoped |
| threads.get_history | `GET /threads/{id}/history` | read | list `snapshots` for the thread |
| threads.create_checkpoint | `POST /threads/{id}/state/checkpoint` | write | INSERT `snapshots` (mirror the worker's `write_checkpoint`) |
| runs.cancel_stateless | `POST /runs/cancel` | write | `run.cancelled` (mirror the existing `cancel` impl, stateless) |
| runs.batch_create | `POST /runs/batch` | write | loop the existing create, one `pg_notify` at end |
| threads.copy | `POST /threads/{id}/copy` | write | read thread's latest state → create a new thread (`thread.created`) |

**Deferred (recorded):**
- `runs.resume` (`run.resumed`) — HITL resume needs interrupts to actually be
  raised by the executor; the counter graph never interrupts, so there is nothing
  to resume. Belongs with the HITL / real-graph-engine work.
- `assistants.get_versions` + `set_latest` — assistant graph *versioning* is thin
  and underspecified today (the `graphs` row carries a version string but there is
  no version history or defined "set latest" semantics). Do it as one coherent
  versioning feature later, not two half-defined stubs now.

## Approach

Two mechanisms, per endpoint:

**Generator impl-mode (add an `impl:` block to `endpoints.yaml`)** where the
endpoint is a single-statement, path-keyed read/write matching an existing mode:
- `get_graph` → `read_one` (row: a new `graphRow` + `toAPI()` → the OpenAPI graph
  type; verify the response type name in `types_gen.go`).
- `get_checkpoint_state` → `read_one` from `snapshots` (reuse `snapshotRow`).
- `get_history` → `read_list` from `snapshots` for the thread.
- `cancel_stateless` → `update` mode emitting `run.cancelled`, mirroring the
  existing `cancel` handler's SQL but keyed on the run id alone (stateless).

**Bespoke `custom` handler** where the logic is multi-statement or shaped
unlike any mode:
- `create_checkpoint` — the same epoch-free INSERT into `snapshots` the worker's
  `write_checkpoint` does, but client-driven (no lease fence: this is an explicit
  client checkpoint of thread state, not a worker execution checkpoint). Resolve
  `stream_id` from the run/thread's event stream.
- `batch_create` — bind a list, loop `run.created` writes + `INSERT runs` in one
  transaction, single `pg_notify` at the end (per `endpoint-queries.d2`), return
  the created run ids.
- `copy` — read the source thread's latest `snapshots` state, create a new thread
  (`thread.created` event + `INSERT threads`) seeded with that state, return the
  new thread.
- `get_state` — latest snapshot across the thread's runs (`SELECT … WHERE
  aggregate_id IN (run ids of thread) ORDER BY version DESC LIMIT 1`), shaped to
  the OpenAPI ThreadState response.

Row/type verification is done in the plan against `types_gen.go` — where an
OpenAPI response type is missing (e.g. the graph or thread-state shape), define a
hand-mapped row `toAPI()` as elsewhere in `rows.go`, recording the divergence.

## Testing

Testcontainers Postgres, no build tag, standard CI. Per group:
- Reads: seed the underlying row(s) (graph / snapshots) + assert the read returns
  them; 404 on missing; thread-scoping (a checkpoint from another thread → 404).
- `create_checkpoint`: write then read-back round-trip; `cancel_stateless`: run →
  cancelled + `run.cancelled` in outbox; `batch_create`: N runs created + N
  `run.created` outbox rows in one tx; `copy`: source thread with state → new
  thread with copied state + `thread.created` outbox row.
- Full `go test ./controlplane/... -count=1` green; generator regen leaves other
  groups byte-identical.

## Files

- `controlplane/gen/endpoints.yaml` — impl blocks for the 4 impl-mode endpoints;
  `custom: true` for the 4 bespoke ones.
- `controlplane/endpoints/runs_gen.go`, `assistants_gen.go`, `threads_gen.go`
  (regenerate).
- `controlplane/endpoints/rows.go` — `graphRow` (+ `toAPI`) if no existing type;
  reuse `snapshotRow`.
- `controlplane/endpoints/threads_state.go` (new) — bespoke thread state/checkpoint
  handlers (`get_state`, `create_checkpoint`, `copy`).
- `controlplane/endpoints/runs_batch.go` (new) — `batch_create`.
- Tests alongside each.

## Out of scope (recorded)

`resume` (HITL), `get_versions`/`set_latest` (versioning) — deferred as above. No
new tables; all target tables (`graphs`, `snapshots`, `threads`, `runs`) exist.
