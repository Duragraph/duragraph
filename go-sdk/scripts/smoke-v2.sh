#!/usr/bin/env bash
# Isolated shipped-binary + Go SDK claim smoke (Docker, Go, curl, jq required).
# Usage: ./go-sdk/scripts/smoke-v2.sh [--transient]
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
name="dg-go-sdk-smoke-$$"
pg="${name}-pg"
nats="${name}-nats"
port="${SMOKE_HTTP_PORT:-18981}"
mkdir -p "$root/go-sdk/bin"
scratch="$(mktemp -d "$root/go-sdk/bin/smoke.XXXXXX")"
server_pid=""
cleanup() {
  local result=$?
  trap - EXIT
  if [ "$result" -ne 0 ] && [ -f "$scratch/server.log" ]; then
    tail -60 "$scratch/server.log" >&2
  fi
  if [ -n "$server_pid" ]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
  docker rm -f "$pg" "$nats" >/dev/null 2>&1 || true
  rm -f "$scratch/duragraph" "$scratch/server.log" "$scratch/result.log"
  rmdir "$scratch"
  exit "$result"
}
trap cleanup EXIT

docker run -d --name "$pg" -e POSTGRES_USER=smoke -e POSTGRES_PASSWORD=smoke -e POSTGRES_DB=smoke -P postgres:16-alpine >/dev/null
docker run -d --name "$nats" -P nats:2.10-alpine -js >/dev/null
pg_port="$(docker port "$pg" 5432/tcp | grep '^0.0.0.0:' | head -1 | cut -d: -f2)"
nats_port="$(docker port "$nats" 4222/tcp | grep '^0.0.0.0:' | head -1 | cut -d: -f2)"
for _ in {1..30}; do
  if docker exec "$pg" pg_isready -U smoke -d smoke >/dev/null 2>&1; then break; fi
  sleep 1
done
docker exec "$pg" pg_isready -U smoke -d smoke >/dev/null

(cd "$root" && go build -o "$scratch/duragraph" ./cmd/duragraph)
env DB_HOST=127.0.0.1 DB_PORT="$pg_port" DB_USER=smoke DB_PASSWORD=smoke DB_NAME=smoke DB_SSLMODE=disable \
  NATS_URL="nats://127.0.0.1:$nats_port" HOST=127.0.0.1 PORT="$port" DURAGRAPH_JWT_SECRET=smoke-only-secret \
  "$scratch/duragraph" serve --control-plane=v2 >"$scratch/server.log" 2>&1 &
server_pid=$!
for _ in {1..50}; do
  if curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/api/v1/workers/runs/00000000-0000-0000-0000-000000000000/graph" | grep -q '^404$'; then break; fi
  if ! kill -0 "$server_pid" 2>/dev/null; then echo 'v2 binary exited before readiness' >&2; exit 1; fi
  sleep 0.2
done

(cd "$root/go-sdk" && go run ./cmd/worker-smoke --url "http://127.0.0.1:$port" "$@") | tee "$scratch/result.log"
run_id="$(sed -n 's/.*PASS terminal run=\([0-9a-f-]*\) status=.*/\1/p' "$scratch/result.log")"
if [ -z "$run_id" ]; then echo 'smoke did not report a terminal run ID' >&2; exit 1; fi
# The public Run projection deliberately omits output. Assert the persisted
# terminal status/output against the throwaway database, not an inferred value.
row="$(docker exec "$pg" psql -U smoke -d smoke -At -c "SELECT json_build_object('status',status,'output',output,'lease_epoch',lease_epoch) FROM runs WHERE id='$run_id'")"
if ! jq -e '.status == "completed" and .lease_epoch == 1 and .output.answer == "smoke-ok" and .output.first == true and .output.input == "hello"' <<< "$row" >/dev/null; then
  echo "unexpected persisted run: $row" >&2
  exit 1
fi
echo "PASS persisted terminal row: $row"
