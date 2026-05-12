# Deploy DuraGraph on Render

Single-container deploy: `duragraph dev` with embedded PostgreSQL + NATS JetStream behind it. No managed Render Postgres add-on, no separate NATS service.

## Prerequisites

- Render account
- GitHub repository connected to Render

## Deploy

### Option 1: Blueprint (recommended)

1. Open the [Render dashboard](https://dashboard.render.com/) → **New** → **Blueprint**.
2. Connect your fork of `github.com/Duragraph/duragraph` (Render needs read access to clone).
3. Render auto-detects `render.yaml`. Review the single service (`duragraph`) and the attached `duragraph-data` disk.
4. **Apply**.

### Option 2: Render CLI

```bash
render blueprint launch --file deploy/render/render.yaml
```

## After deploy

- Visit the service URL Render assigns and go to `/register`. The first user to register is auto-promoted to admin. No default credentials — choose your own email and password.
- (Optional) Set `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` in the service's **Environment** tab if you want assistants backed by real models.
- (Optional) Flip `AUTH_PASSWORD_ENABLED` to `true` to require email+password login — first registered user becomes the admin.

## What `render.yaml` configures

- One Docker web service, built from `deploy/docker/Dockerfile.server`.
- Service CMD overridden to `./duragraph dev --port $PORT --data-dir /data`.
- 5 GB persistent disk mounted at `/data` (embedded Postgres + NATS state).
- Healthcheck on `/health`.

## Scaling considerations

`duragraph dev` mode runs Postgres + NATS in-process, so the service is single-instance by design. For horizontal scaling switch to `duragraph serve` against external Postgres + NATS — outside the scope of this template.

## Update

Push to your `main`; Render rebuilds and rolls out automatically. The disk survives image swaps.

## Troubleshooting

**Disk filling up** — bump `sizeGB` in `render.yaml`; Render expands the disk in place.

**Service unhealthy** — check the **Logs** tab. First boot is slow (the embedded Postgres has to extract + initdb), and Render's default healthcheck grace is short; bump `initialDelaySeconds` on the service if cold starts on the `starter` plan exceed it.
