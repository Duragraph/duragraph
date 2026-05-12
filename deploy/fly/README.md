# Deploy DuraGraph on Fly.io

Single-container deploy: `duragraph dev` running with embedded PostgreSQL + NATS JetStream behind it. No managed-Postgres add-on, no separate NATS service.

## Prerequisites

- [Fly CLI](https://fly.io/docs/hands-on/install-flyctl/) installed
- Fly.io account

## Deploy

```bash
# 1. Authenticate
flyctl auth login

# 2. Create the app (don't deploy yet — we need to create the volume first)
flyctl launch --no-deploy --copy-config

# 3. Create a persistent volume for the embedded data plane
flyctl volumes create duragraph_data --size 10 --region iad

# 4. (Optional) Set LLM provider secrets
flyctl secrets set OPENAI_API_KEY=sk-...
flyctl secrets set ANTHROPIC_API_KEY=sk-...

# 5. (Optional) Turn on password auth — the first user to register
#    becomes the bootstrap admin
flyctl secrets set AUTH_PASSWORD_ENABLED=true

# 6. Deploy
flyctl deploy
```

Then open the URL `flyctl status` prints and visit `/register`. The first user to register is auto-promoted to admin. No default credentials ship with the binary — choose your own email and password.

## What `fly.toml` configures

- Single machine, single container, image built from `deploy/docker/Dockerfile.server`.
- CMD overridden to `./duragraph dev --port 8080 --data-dir /data` — embedded Postgres + NATS, single-tenant.
- Persistent volume `duragraph_data` mounted at `/data` — holds the embedded Postgres data directory and NATS JetStream streams. Survives machine restarts and image updates.
- HTTP service on port 8080 with `/health` check (30 s grace, 30 s interval).
- `auto_stop_machines = false` and `min_machines_running = 1` — embedded state means the machine should not be paused; restarting it cold loses no data but does cost recovery time on the embedded Postgres side.

## Scaling considerations

`duragraph dev` mode runs Postgres + NATS in-process, so horizontal scaling beyond one machine is not supported in this configuration. For multi-instance / HA deploys, switch to `duragraph serve` against external Postgres + NATS — outside the scope of this template.

## Update

```bash
flyctl deploy
```

Rolls out a new image and re-attaches the existing volume. The embedded data plane survives.

## Troubleshooting

**Volume size too small** — embedded Postgres can grow with usage; bump with `flyctl volumes extend <id> --size <gb>`.

**App not reachable** — check `flyctl logs` for the bootstrap admin credentials line, and confirm `/health` returns 200 via `curl https://<app>.fly.dev/health`.
