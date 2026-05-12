# Deploy DuraGraph on Scaleway Serverless Containers

Single-container deploy: `duragraph dev` with embedded PostgreSQL + NATS JetStream behind it. No managed Postgres database to provision.

## Prerequisites

- Scaleway account with Serverless Containers enabled
- [scw CLI](https://github.com/scaleway/scaleway-cli) installed
- A container registry namespace (e.g. `scw registry namespace create`)

## Deploy

```bash
# 1. Build + push the image to your Scaleway registry
scw registry login
docker build -f deploy/docker/Dockerfile.server \
  -t rg.fr-par.scw.cloud/<namespace>/duragraph:latest .
docker push rg.fr-par.scw.cloud/<namespace>/duragraph:latest

# 2. Reference that image in scaleway.yaml (update the image.name field
#    to point at your registry path)

# 3. Deploy via the Scaleway console or the Terraform / Pulumi tooling
#    of your choice — Scaleway does not currently accept the YAML spec
#    as a direct CLI input; treat scaleway.yaml as documentation of the
#    intended container shape.
```

## What `scaleway.yaml` describes

- Single container, public on port 8080.
- CMD overridden to `./duragraph dev --port 8080 --data-dir /data`.
- 10 GB local-SSD volume mounted at `/data` for embedded Postgres + NATS state.
- Healthcheck on `/health` (30 s interval, 5 s timeout).
- `min_scale: 1`, `max_scale: 1` — embedded data plane pins this to a single instance.
- Optional secrets: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `AUTH_PASSWORD_ENABLED`, `AUTH_OAUTH_ENABLED`.

## After deploy

- Hit the public URL Scaleway assigns and visit `/register`. The first user to register is auto-promoted to admin. No default credentials — choose your own email and password.

## Scaling considerations

Same as the other single-container templates: embedded Postgres prevents horizontal scaling. Switch to `duragraph serve` against external Postgres + NATS for HA workloads.

## Troubleshooting

**Local-SSD volumes are tied to a single container instance.** If the scheduler moves the container to a different node, state is lost. For production, use a Block Storage volume or a managed Postgres backend (i.e. the production `duragraph serve` deploy path, not this template).
