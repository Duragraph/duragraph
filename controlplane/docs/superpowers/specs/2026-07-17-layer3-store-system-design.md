# Layer 3 stubs — Pass 1: `store` + `system`

**Date:** 2026-07-17
**Branch:** `feat/controlplane-server`
**Status:** approved design, pending implementation plan

## Context

The control-plane rebuild (`controlplane/`) builds bottom-up from the d2 diagrams
in `spec/models/d2/` — the source of truth. Layers 1–4 are done (schema
migrations, transactional outbox, endpoint codegen, server assembly, NATS).
Layer 3 (endpoints) is generated from `controlplane/gen/endpoints.yaml` (the
machine form of `endpoint-queries.d2`): an endpoint with an `impl:` block gets a
real handler body; without one it gets a TODO scaffold.

Four endpoint groups are implemented (`assistants`, `threads`, `runs`, `crons`).
Six remain stubbed. This design covers **Pass 1** of finishing them.

### Scope

Ships in this pass:

- **`store`** — full: put, get, delete, search, list_namespaces.
- **`system`** — `/ok`, `/info`, `/metrics`.

Deferred to later passes (explicit, out of scope here):

- **`workers`, `admin`, `platform`** — DuraGraph-native contracts that do **not**
  exist in `spec/api/duragraph-latest.yaml`. The spec-first rule requires a spec
  change before implementation, so these are a **Pass 2 spec-first effort**:
  add the native worker/admin/platform contracts to the API spec, regenerate
  types, then implement. `platform/me` additionally depends on JWT/auth
  middleware that does not yet exist.
- **`auth`** — its own subsystem (goth OAuth + password + sessions/JWT). Separate
  spec.

### Why store + system are unblocked

- `store` is part of the LangGraph Cloud API: OpenAPI types already exist in
  `types_gen.go` (`StorePutRequest`, `StoreSearchRequest`, `StoreDeleteRequest`,
  `StoreListNamespacesRequest`, `Item`, `SearchItemsResponse`,
  `ListNamespaceResponse`). Tables `store_items` + `store_namespaces` exist
  (migration `004_store`).
- `system` `/ok` and `/info` paths exist in `duragraph-latest.yaml`. `/metrics`
  is a standard Prometheus scrape endpoint. No DB contract needed.

## Architecture

### Generator `custom` flag — the reusable mechanism

The existing impl modes (`write_returning`, `read_one`, `update`, `delete`,
`hard_delete`, `read_list`, `count`, `write_plain_returning`) all express a
**single-statement, UUID-path-keyed** projection. Many remaining endpoints do
not fit that shape (composite keys, 204 returns, wrapped responses,
multi-statement writes). Rather than grow the template into control flow it is
bad at, we add one escape hatch:

- New endpoint field in `endpoints.yaml`: **`custom: true`**.
- In `templates/group.go.tmpl`: when an endpoint is `custom`, the generator emits
  the **route registration only** (`g.VERB("path", s.Handler)` inside
  `Register<Group>`) and **skips emitting the method body**.
- The handler method (`func (s *Server) <Group><Endpoint>(c echo.Context) error`)
  is **hand-written** in a sibling file (`<group>.go`, same `package endpoints`).
  Go resolves it at compile time across files; no method-name collision.
- Import generation stays correct: a `custom` endpoint sets none of the
  `Needs{Pgx,UUID,Errors}` flags, so a fully-custom generated file imports only
  `echo`.

This keeps routing single-sourced (generated from the manifest) while letting
bespoke bodies be hand-written. Pass 2's multi-statement worker/admin handlers
(over the existing `writeTx` helper) will reuse this exact flag.

### `store` — hand-written over the plain tenant pool

`store` is **not** event-sourced (no events/outbox/`writeTx`), so handlers run
plain `s.Tenant` pool queries. All five endpoints are `custom: true`; bodies live
in a new `controlplane/endpoints/store.go`. A `storeItemRow` struct + `toAPI()`
mapper go in `rows.go` alongside the existing rows.

Schema decision — **`namespace` is `TEXT[]`** (postgres.d2; OpenAPI exposes it as
`[]string`). The illustrative `namespace LIKE :prefix || '%'` SQL in
`endpoint-queries.d2` predates the `TEXT[]` decision and is **discarded** in
favor of Postgres array operators. This divergence is recorded in the rows.go
DIVERGENCES block and reconciled in `endpoint-queries.d2` (paired spec touch or
`[spec-skip]` trailer with reason).

| Endpoint | Method / path | Query (against `store_items`, `namespace TEXT[]`) | Response |
|----------|---------------|---------------------------------------------------|----------|
| put | `PUT /store/items` | `INSERT (namespace,key,value) VALUES ($1,$2,$3) ON CONFLICT (namespace,key) DO UPDATE SET value=$3, updated_at=now()` | `204 No Content` |
| get | `GET /store/items?namespace=&key=` | `SELECT … WHERE namespace=$1 AND key=$2` | `Item` / `404` |
| delete | `DELETE /store/items` (body `StoreDeleteRequest`) | `DELETE WHERE namespace=$1 AND key=$2` | `204` (or `404` if absent) |
| search | `POST /store/items/search` (`StoreSearchRequest`) | `SELECT … WHERE namespace[:len]=$prefix (array prefix) AND value @> $filter ORDER BY … LIMIT $limit OFFSET $offset` | `SearchItemsResponse{items:[]Item}` |
| list_namespaces | `POST /store/namespaces` (`StoreListNamespacesRequest`) | `SELECT DISTINCT namespace … ` filtered by prefix/suffix/max_depth | `ListNamespaceResponse` (`[][]string`) |

Notes:
- `namespace` on GET arrives as a query param. LangGraph passes the namespace as
  a single `.`-joined path in the `namespace` query arg; the handler splits it
  into the `[]string` used for the `TEXT[]` equality. Exact param encoding is
  confirmed against `duragraph-latest.yaml` during implementation.
- Array **prefix** match uses `namespace[1:array_length($prefix,1)] = $prefix`
  (or `$prefix <@ namespace` where prefix semantics permit); the precise operator
  is chosen and unit-covered in implementation. `value @> $filter` uses the
  existing GIN `jsonb_path_ops` index.
- `max_depth`/`suffix` in list_namespaces are honored where cheaply expressible;
  anything not honored is recorded in DIVERGENCES rather than silently dropped.

### `system` — hand-written, mounted on root

`system` is fully bespoke with no DB contract. It is **removed from the generator
manifest** (delete the `system` group from `endpoints.yaml`; delete the generated
`system_gen.go`) and hand-written in `controlplane/endpoints/system.go`.

`RegisterSystem(e *echo.Echo)` mounts on the **root** Echo instance (not the
`/api/v1` group), because the spec paths are root-level (`/ok`, `/info`,
`/metrics`) and the dashboard auth gate (#208) expects `/info` at root.

| Endpoint | Behavior |
|----------|----------|
| `GET /ok` | `s.Tenant.Ping(ctx)` → `200 {"status":"ok"}` or `503`. NATS liveness is a documented follow-up: `endpoints.Server` holds no NATS handle today. |
| `GET /info` | `{"version","git_sha","uptime_seconds"}`. `version`/`git_sha` are package vars set via `-ldflags -X`; uptime derives from a start time set in server `New`. |
| `GET /metrics` | `echo.WrapHandler(promhttp.Handler())` over the default Prometheus registry (`prometheus/client_golang` already in `go.mod`). |

`server.go` change: replace `ep.RegisterSystem(g)` with `ep.RegisterSystem(e)`
mounted before the `/api/v1` group; pass/record the process start time so `/info`
uptime is real.

## Data flow

- **store** — client → Echo `/api/v1/store/*` → `store.go` handler → `s.Tenant`
  pool query → row → `toAPI()` → JSON. No events, no outbox, no NATS.
- **system** — client → Echo root `/ok|/info|/metrics` → `system.go` handler →
  (DB ping | in-memory build info | Prometheus registry). No tenant/domain state.

## Error handling

- Follows the established handler convention: `echo.NewHTTPError(status, msg)`;
  `pgx.ErrNoRows` → `404`; bind failures → `400`; unexpected DB errors → `500`.
- store get/delete on a missing `(namespace,key)` → `404` (get) / `404` (delete
  when nothing removed), consistent with `hard_delete` semantics elsewhere.
- `/ok` returns `503` when the DB ping fails so it is usable as a readiness probe.

## Testing

Testcontainers integration tests in CI (project default — no build tag), mirroring
`crons_integration_test.go` and reusing the shared `TestMain` Postgres container:

- `store_integration_test.go` — put→get roundtrip; put upsert overwrites; get 404;
  delete then get 404; search by namespace prefix + `value @>` filter (positive +
  negative); list_namespaces prefix filtering; verifies `TEXT[]` array semantics.
- `system_integration_test.go` — `/ok` 200 with DB up; `/info` returns the three
  fields with a non-empty/plausible uptime; `/metrics` 200 and Prometheus text
  format. Confirms system routes are reachable at **root**, not `/api/v1`.

Green tests are the done-criteria and prove the `custom`-flag coexistence pattern
(generated routes + hand-written bodies) that Pass 2 depends on.

## Files touched

- `controlplane/gen/main.go` — parse `custom` on `endpoint`; suppress `Needs*` for
  custom endpoints.
- `controlplane/gen/templates/group.go.tmpl` — emit route-only for `custom`
  endpoints; skip body.
- `controlplane/gen/endpoints.yaml` — mark all `store` endpoints `custom: true`;
  remove the `system` group.
- `controlplane/endpoints/store.go` — new: 5 hand-written store handlers.
- `controlplane/endpoints/store_gen.go` — regenerated: `RegisterStore` routes only.
- `controlplane/endpoints/system.go` — new: hand-written system handlers +
  `RegisterSystem(e)`.
- `controlplane/endpoints/system_gen.go` — deleted.
- `controlplane/endpoints/rows.go` — add `storeItemRow` + `toAPI()`; extend
  DIVERGENCES with the store-namespace `TEXT[]` note.
- `controlplane/server/server.go` — mount `RegisterSystem` on root `e`; record
  process start time for `/info`.
- `controlplane/endpoints/store_integration_test.go`,
  `system_integration_test.go` — new.
- `spec/models/d2/endpoint-queries.d2` — reconcile store SQL to `TEXT[]` array ops
  (paired spec change) OR `[spec-skip]` trailer with reason.

## Out of scope (recorded so it is not silently dropped)

- NATS liveness in `/ok` (needs a NATS handle on `endpoints.Server`).
- `workers`, `admin`, `platform` groups (Pass 2, spec-first).
- `platform/me` JWT extraction (rides with auth).
- `auth` group (separate subsystem/spec).
- store vector/semantic `query` field in `StoreSearchRequest` (no vector index in
  the new control plane yet) — recorded as a DIVERGENCE, filter/prefix only.
