# Deploy DuraGraph on Railway

Single-container deploy: `duragraph dev` with embedded PostgreSQL + NATS JetStream behind it. No second Postgres service to provision.

## Prerequisites

- Railway account
- [Railway CLI](https://docs.railway.com/develop/cli) (optional, for command-line deploys)
- A fork of `github.com/Duragraph/duragraph` connected to Railway

## Deploy

### Option 1: Railway dashboard

1. Open the [Railway dashboard](https://railway.com/) → **New Project** → **Deploy from GitHub repo**.
2. Pick your fork; Railway auto-detects `railway.toml`.
3. Add a **Volume** to the service in the dashboard, mounted at `/data` (10 GB is plenty for the embedded data plane).
4. (Optional) Set `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` in the service's **Variables** tab.
5. **Deploy**.

### Option 2: Railway CLI

```bash
railway link <project>
railway up
# then attach a volume + set env vars via `railway variables`
```

## What `railway.toml` configures

- Single service, image built from `deploy/docker/Dockerfile.server`.
- `startCommand`: `./duragraph dev --port $PORT --data-dir /data` — Railway injects `$PORT` at runtime.
- Healthcheck on `/health`.
- `restartPolicyType: ON_FAILURE` with up to 10 retries.

## After deploy

- Open the public URL Railway assigns; sign in with the bootstrap admin credentials in the logs (**Deploys → Logs**).
- (Optional) Flip `AUTH_PASSWORD_ENABLED` to `true` via env vars to enable login.

## Scaling considerations

Embedded Postgres pins the service to a single replica. For horizontal scaling switch to `duragraph serve` against external Postgres + NATS.

## Update

Push to `main`; Railway redeploys automatically. The attached volume survives the rebuild.

## Troubleshooting

**Volume not attached** — by default Railway services have no persistent storage. You must add a volume in the dashboard (Settings → Volumes) and mount it at `/data` before the first run, otherwise embedded state evaporates on every redeploy.

**Cold start slow** — first boot extracts the embedded Postgres binary and `initdb`s. Subsequent restarts skip both. Default healthcheck timeout (30s in the manifest) covers this.
