# Deploy DuraGraph on DigitalOcean App Platform

Deploy DuraGraph to DigitalOcean's App Platform with managed PostgreSQL and NATS.

## Prerequisites

- DigitalOcean account
- [doctl CLI](https://docs.digitalocean.com/reference/doctl/how-to/install/) (optional)

## Quick Deploy

### Option 1: One-Click Deploy

[![Deploy to DO](https://www.deploytodo.com/do-btn-blue.svg)](https://cloud.digitalocean.com/apps/new?repo=https://github.com/Duragraph/duragraph/tree/main)

### Option 2: Manual Deploy via UI

1. Go to [DigitalOcean Apps](https://cloud.digitalocean.com/apps/new)
2. Connect your GitHub repository
3. Select the branch to deploy
4. DigitalOcean will auto-detect the app spec from `.do/app.yaml`
5. Review the configuration
6. Click "Create Resources"

### Option 3: Deploy via CLI

```bash
# Install doctl
brew install doctl  # macOS
# or snap install doctl  # Linux

# Authenticate
doctl auth init

# Create app from spec
doctl apps create --spec deploy/digitalocean/.do/app.yaml

# Get app ID
doctl apps list

# Set secrets
doctl apps create-deployment <app-id> --env OPENAI_API_KEY=your-key-here
doctl apps create-deployment <app-id> --env ANTHROPIC_API_KEY=your-key-here
doctl apps create-deployment <app-id> --env JWT_SECRET=$(openssl rand -hex 32)
```

## Architecture

On DigitalOcean, DuraGraph runs with:

- **API Service**: DuraGraph server (containerized)
- **NATS Service**: NATS JetStream (containerized)
- **Dashboard Service**: Svelte dashboard (containerized)
- **Database**: Managed PostgreSQL 15

## Environment Variables

Configure these in the DigitalOcean App Platform dashboard under "Settings" → "App-Level Environment Variables":

### Required
- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` - Auto-configured from managed database
- `NATS_URL` - Set to `nats://nats:4222`

### Optional
- `OPENAI_API_KEY` - For OpenAI integration
- `ANTHROPIC_API_KEY` - For Anthropic integration
- `JWT_SECRET` - Required if AUTH_ENABLED=true
- `AUTH_ENABLED` - Set to "true" to enable authentication

## Scaling

```bash
# Scale API service
doctl apps update <app-id> --spec updated-app.yaml

# Or via UI: Apps → Your App → Settings → Scale
```

## Monitoring

- View logs: Apps → Your App → Runtime Logs
- Metrics: Apps → Your App → Insights
- Alerts: Apps → Your App → Settings → Alerts

## Custom Domain

1. Go to Apps → Your App → Settings
2. Click "Domains"
3. Add your custom domain
4. Update DNS records as instructed

## Cost Estimate

- **Basic plan**: $5/month per service (~$15/month for all services)
- **PostgreSQL**: $15/month for dev database
- **Total**: ~$30/month for development setup

[DigitalOcean Pricing](https://www.digitalocean.com/pricing/app-platform)

## Updating Configuration

Edit `.do/app.yaml` and push to your repository. DigitalOcean will automatically deploy changes.

## Troubleshooting

### Database Connection Issues
- Ensure `DB_SSLMODE=require` is set
- Check that the database is in the same region as your app

### NATS Connection Issues
- Verify NATS service is healthy
- Check internal networking between services
