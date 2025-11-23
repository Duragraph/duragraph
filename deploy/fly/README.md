# Deploy DuraGraph on Fly.io

Deploy DuraGraph to Fly.io with PostgreSQL and NATS.

## Prerequisites

- [Fly CLI](https://fly.io/docs/hands-on/install-flyctl/) installed
- Fly.io account

## Quick Deploy

```bash
# Login to Fly.io
flyctl auth login

# Create a new app
flyctl launch --no-deploy

# Create PostgreSQL database
flyctl postgres create --name duragraph-db --region iad

# Attach database to app
flyctl postgres attach duragraph-db

# Set secrets
flyctl secrets set OPENAI_API_KEY=your-key-here
flyctl secrets set ANTHROPIC_API_KEY=your-key-here
flyctl secrets set JWT_SECRET=$(openssl rand -hex 32)

# Deploy
flyctl deploy
```

## Architecture

On Fly.io, DuraGraph runs with:

- **App**: DuraGraph API server (Go)
- **Database**: Fly Postgres (PostgreSQL 15)
- **NATS**: Deployed as sidecar container

## Environment Variables

Set these using `flyctl secrets set`:

- `DB_HOST` - Auto-configured by Fly Postgres
- `DB_PORT` - Auto-configured by Fly Postgres
- `DB_USER` - Auto-configured by Fly Postgres
- `DB_PASSWORD` - Auto-configured by Fly Postgres
- `DB_NAME` - Auto-configured by Fly Postgres
- `NATS_URL` - Set to `nats://localhost:4222` (sidecar)
- `OPENAI_API_KEY` - Optional, for OpenAI integration
- `ANTHROPIC_API_KEY` - Optional, for Anthropic integration
- `JWT_SECRET` - Required if AUTH_ENABLED=true

## Scaling

```bash
# Scale to multiple machines
flyctl scale count 2

# Scale machine size
flyctl scale vm shared-cpu-2x --memory 2048
```

## Monitoring

```bash
# View logs
flyctl logs

# Check status
flyctl status

# Open dashboard
flyctl dashboard
```

## Custom Domain

```bash
flyctl certs add yourdomain.com
```

## Cost Estimate

- **Free tier**: 3 shared-cpu-1x VMs (256MB RAM)
- **Postgres**: ~$2/month for hobby tier
- **Additional compute**: ~$0.0000008/second per machine

[Fly.io Pricing](https://fly.io/docs/about/pricing/)
