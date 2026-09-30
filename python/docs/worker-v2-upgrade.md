# Worker protocol v2 upgrade

The Python worker now requires a control plane exposing `POST /api/v1/workers/register`,
`POST /api/v1/workers/{id}/runs/claim`, and `POST /api/v1/workers/{id}/runs/{rid}/events`.
It **does not** call the retired `/poll` or worker-wide `/events` routes. Upgrade the
server first; old deployments cannot serve this worker. Local graph execution is unaffected.

```python
worker = Worker("http://localhost:8081", name="agent-1", claim_interval=1.0)
worker.register_graph(agent._get_definition(), instance=agent)
worker.run()
```

- Replace `poll_interval` with `claim_interval`; this is the cadence for **leasing**
  queued work, not the old poll API. The SDK claims only available capacity.
- Registration sends UUID `worker_id`, `graphs`, `capacity`, and graph definitions.
  `name` is used to derive a stable UUID for re-registration. Use a distinct name
  per concurrent worker process; identical names share a lease and may interfere.
- Claim returns `runs` with `run`, `graph_id`, `input`, `lease_epoch`, and optional
  `checkpoint_id`. An empty queue is `{"runs":[]}`. Claim itself records `run.started`:
  **do not** post another start event, which would increment the epoch again.
- Events are sent to the per-run route in `{"events":[{"type":..., "lease_epoch":...}]}`.
  Node lifecycle names are `execution.node_started` / `execution.node_completed`;
  terminal names are `run.completed`, `run.failed`, `run.requires_action`.
  HTTP 409 on an event/checkpoint fences the stale worker; it must stop reporting
  that run. Event writes are not best-effort.
- After a node, a threaded run writes a checkpoint containing `channels` and
  `next_node`. A claimed `checkpoint_id` loads this state to resume. The first
  node of an old uncheckpointed run can be re-executed after a crash; make node
  side effects idempotent. Runs without a thread do not write checkpoints.
- The old NATS notification path is not used by this claim-based worker.
  Passing `nats_url` raises `ValueError` instead of silently falling back.
  Remove `nats_url` / `--nats-url` from deployment configuration.
- Shutdown drains active runs, then deregisters the worker. If draining times
  out, in-progress runs may be requeued; side effects must tolerate replay.
