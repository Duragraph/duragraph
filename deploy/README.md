# DuraGraph Deployment Templates

Five one-click deploy templates for getting `duragraph dev` running on a public cloud. Each is a **single container** that boots the engine with embedded PostgreSQL + NATS JetStream — no managed database, no sidecar broker, no separate dashboard service. State persists on a volume mounted at `/data`.

These templates are designed for **try-it-out / single-tenant production**. For horizontal scaling, multi-tenancy, or HA, run `duragraph serve` against external Postgres + NATS — outside the scope of these templates.

## Available platforms

| Platform | Best for | Setup time | Guide |
|---|---|---|---|
| **Fly.io** | Global edge, low latency, persistent volumes | 5 min | [./fly/](./fly/README.md) |
| **Render** | Easy git-based deploys, managed disks | 5 min | [./render/](./render/README.md) |
| **Railway** | Best DX, PR previews, generous free tier | 3 min | [./railway/](./railway/README.md) |
| **DigitalOcean** | Simple, predictable pricing | 10 min | [./digitalocean/](./digitalocean/README.md) |
| **Scaleway** | European data residency, serverless containers | 10 min | [./scaleway/](./scaleway/README.md) |

## What every template runs

```
┌──────────────────────────────────────────────┐
│  DuraGraph (single binary, single container) │
├──────────────────────────────────────────────┤
│  Go API + SSE      :8080  (HTTP)             │
│  React dashboard          (same origin)      │
│  Embedded Postgres :5435  (in-process child) │
│  Embedded NATS     :4222  (in-process)       │
├──────────────────────────────────────────────┤
│  Persistent volume → /data                   │
│    pg/    embedded Postgres data directory   │
│    nats/  JetStream stream + consumer state  │
└──────────────────────────────────────────────┘
```

The container CMD is overridden to `./duragraph dev --port <port> --data-dir /data` on every platform. Healthcheck is `GET /health`.

## Environment variables

All optional — sensible defaults are baked in. Override via the platform's secrets / env vars panel.

```bash
# LLM provider keys. If absent, assistants fall back to a rule-based mock.
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-...

# Bind address (set by templates already, listed here for completeness)
HOST=0.0.0.0

# Three-flag auth split. Off by default (single-tenant try-it-out mode).
# First registered user becomes the bootstrap admin when password auth is on.
AUTH_PASSWORD_ENABLED=true|false
AUTH_OAUTH_ENABLED=true|false
MULTITENANT_ENABLED=true|false
```

## Local test before deploying

```bash
# Run the same image the platforms use, locally
docker build -f deploy/docker/Dockerfile.server -t duragraph:local .
docker run --rm -p 8080:8080 -v duragraph-data:/data duragraph:local \
  ./duragraph dev --port 8080 --data-dir /data
```

Open `http://localhost:8080`, sign in with the bootstrap admin credentials printed on first boot.

## Verifying after deploy

```bash
curl https://<your-app-url>/health         # → 200 {"status":"healthy",...}
curl https://<your-app-url>/api/v1/assistants
open  https://<your-app-url>               # dashboard is on the same origin
```

## Scaling

These templates pin themselves to a **single instance** because the data plane is embedded — Postgres can't safely have two writers, NATS JetStream's stream files don't tolerate concurrent processes. Adding a second instance loses data.

For horizontal scaling switch to `duragraph serve` against external Postgres + NATS. None of these templates cover that path; bring your own production deploy or use the engine's Helm chart (in flight under `deploy/helm/`).

## Troubleshooting

**Service won't start**
- Check build logs — the most common breakage is the image build failing because the dashboard wasn't pre-built. Make sure the platform builds from the repo root with `deploy/docker/Dockerfile.server` (multi-stage; builds the dashboard inside the Docker build).

**Healthcheck failing on first boot**
- First boot is slow: the embedded Postgres library extracts its binary (~20 MB) and runs `initdb`. 20–30 seconds is normal. Each platform's template uses an `initialDelaySeconds`-equivalent of 30 s; bump it if your instance class is slower than that.

**Volume not attached**
- DigitalOcean and Railway require attaching the volume via their UI; the YAML spec alone doesn't provision storage. Without a volume mounted at `/data`, the embedded data plane evaporates on every redeploy.

## Getting help

- [GitHub Issues](https://github.com/Duragraph/duragraph/issues) — bug reports
- [GitHub Discussions](https://github.com/Duragraph/duragraph/discussions) — questions, ideas
- [duragraph.ai/docs](https://duragraph.ai/docs) — full docs
