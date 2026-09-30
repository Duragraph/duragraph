# Go worker migration: claim / push protocol

The worker protocol is the **new** control-plane worker implementation, but its
implemented routes are under `/api/v1` (see `controlplane/endpoints/workers_gen.go`).
There is no `/api/v2` worker route and no compatibility `/poll` adapter.

## Migration

1. Deploy a control plane with `POST /api/v1/workers/register`, `POST
   /api/v1/workers/{id}/runs/claim`, `POST
   /api/v1/workers/{id}/runs/{rid}/events`, `GET
   /api/v1/workers/runs/{rid}/graph`, and thread checkpoint routes. An old
   control plane with `/poll`, `/events`, `/complete` will not work.
2. Replace `worker.WithPollInterval(d)` with `worker.WithClaimInterval(d)`.
   Without `WithNATS`, the worker claims **one available run per free slot**.
   An empty claim response is `{ "runs": [] }`, not `{ "tasks": [] }`.
3. `WithNATS(url)` selects **push instead of claim**: connect to an existing
   `WORKER_COMMANDS` stream and `graph-executor` durable pull consumer. It
   does not fall back to claim on NATS failure. Make sure the worker can use
   the same tenant NATS account as the control plane.
4. Registration now sends a UUID `worker_id`, `graphs`, `capacity`, and
   `graph_definitions` with `{name,nodes,edges,config}`. `WithName` is a local
   label, not the protocol ID. Heartbeat sends `status:"online"` and
   `active_runs`; a 409 prompts re-registration. On shutdown the worker
   deregisters after consumers stop.
5. For claimed work, **do not post `run.started` again**: claim already leased
   the run and returned `lease_epoch` (and optional `checkpoint_id`). For
   push commands (`worker.graph.execute`) post `run.started` and use its
   returned epoch. Each node event, checkpoint, and terminal event carries
   this epoch; a 409 fences the old executor. The graph is fetched by run ID
   and compared with the local graph before executing it. Per-node snapshots
   store `{state,next}`; redelivery resumes from the latest checkpoint.
   Local Go `function` nodes are reported as `tool`, and routers as
   `conditional`, because `execution_history.node_type` accepts only
   `start|end|llm|tool|conditional|human` (the registered graph retains its
   original node types).

## Shipped-binary end-to-end smoke

With Docker, Go, curl and jq available, from the repository root run:

```bash
bash go-sdk/scripts/smoke-v2.sh --transient
```

The script builds the actual `./cmd/duragraph` release entrypoint, starts
fresh temporary Postgres and NATS containers on random published ports, and
launches `duragraph serve --control-plane=v2` on `127.0.0.1:18981` (override
with `SMOKE_HTTP_PORT`). It runs a **Go SDK worker**, registers a two-node
graph, creates an assistant/thread/run through the HTTP API, claims and
executes the run, and checks the API terminal status, checkpoint version/state,
and the persisted `runs.output` and lease epoch in the isolated database. The
optional `--transient` injects one 503 at the SDK's checkpoint HTTP transport
after the second run has already been leased; the run remains `running` in the
API while the worker lives, since HTTP claim selects only queued runs. On
worker shutdown, deregistration requeues that run. The script removes **only
its own** temporary containers and files on exit. Do not point the Go helper
at a production or shared control plane.

The public run API reports DB `completed` as `success` and intentionally
omits `runs.output`; `client.WaitForRun` accepts these v2 terminal statuses,
but callers needing persisted output currently need a separate read path.

## Important limitations / rollout gates

- The existing `graph-executor` durable consumer is shared across **all**
  graphs. A Go worker with `WithNATS` cannot subscribe to just its own graph.
  A deployment with heterogeneous worker graphs must partition consumers or
  provide a dispatch filter on the server before push mode is safe. Do not
  enable push mode on such deployments yet. A mismatched graph is rejected,
  not executed.
- HTTP claim leases a queued run immediately; it cannot reclaim an in-progress
  run. If a transient error occurs after claim, the current worker logs it
  but cannot safely replay it. Deregistering requeues in-progress work; until
  then the run remains leased and stuck. Use
  push mode where correctly partitioned and provisioned, or add a server-side
  requeue/recovery mechanism before relying on claim for durable execution.
- The Go graph is a locally compiled sequential graph. Its checkpoint format
  is **not interchangeable** with `controlplane/worker`'s graph interpreter.
  It currently does not implement HITL `resume`, cross-run checkpoint seeds,
  run-level kwargs, dynamic graph definitions, or `max_iterations`. A
  `run.resumed` push command fails closed rather than silently treating it as
  a fresh run. For those workflows use the native control-plane runner.
- Checkpoints are written after node execution and before `node_completed`.
  A crash between execution and checkpoint may repeat a node. Make node side
  effects idempotent. Terminal reporting errors are retried in push mode via
  JetStream NAK; HTTP claim mode cannot do this after losing its lease slot.
- The worker's UUID is generated per process. Keep worker lifetime bounded by
  its context; registration retry is not automatic on initial failure.

The REST `client` package's `WaitForRun` polls a **user-facing run resource**;
it is unrelated to the retired worker `/poll` route.
