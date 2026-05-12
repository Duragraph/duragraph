# Deploy DuraGraph on DigitalOcean App Platform

Single-container deploy: `duragraph dev` with embedded PostgreSQL + NATS JetStream behind it. No managed dev database to provision.

## Prerequisites

- DigitalOcean account
- [doctl](https://docs.digitalocean.com/reference/doctl/) installed (optional, for CLI deploys)
- A fork of `github.com/Duragraph/duragraph` accessible to App Platform

## Deploy

### Option 1: App Platform dashboard

1. Open [Apps](https://cloud.digitalocean.com/apps) → **Create App**.
2. Choose **GitHub** as the source, select your fork.
3. App Platform reads `deploy/digitalocean/.do/app.yaml`. Review the single service.
4. **Important**: attach a Volume to the service (App Platform → Components → api → Resources → Add Volume), mounted at `/data`. 10 GB is comfortable.
5. (Optional) Set `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` as encrypted environment variables.
6. **Create Resources**.

### Option 2: doctl

```bash
doctl apps create --spec deploy/digitalocean/.do/app.yaml
```

## What `.do/app.yaml` configures

- Single Docker service, built from `deploy/docker/Dockerfile.server`.
- `run_command`: `./duragraph dev --port 8080 --data-dir /data`.
- HTTP port 8080.
- Healthcheck on `/health` with 30s initial delay (embedded Postgres needs that on cold start).

## After deploy

- Visit the public URL App Platform assigns. Sign in with the bootstrap admin credentials in the **Runtime Logs** of the api component.
- (Optional) Flip `AUTH_PASSWORD_ENABLED` to `true` in env vars to require email+password login.

## Scaling considerations

Embedded Postgres pins this to `instance_count: 1`. For horizontal scaling switch to `duragraph serve` against a managed DigitalOcean Postgres + an external NATS — not covered by this template.

## Update

Push to `main`; App Platform redeploys automatically (`deploy_on_push: true`).

## Troubleshooting

**Volume attach via UI only** — volumes can only be attached via the App Platform UI for the App; the YAML spec doesn't (yet) declare them. Add it manually after the first deploy.

**Healthcheck failing during cold start** — first boot extracts the embedded Postgres binary; can take 20–30 seconds. The 30 s `initial_delay_seconds` should cover it. If your image takes longer on a basic-xxs instance, bump that value.
