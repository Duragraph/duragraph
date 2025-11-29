# Deploy DuraGraph on Render

Deploy DuraGraph to Render with managed PostgreSQL and NATS.

## Prerequisites

- Render account
- GitHub repository connected to Render

## Quick Deploy

### Option 1: One-Click Deploy

[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/Duragraph/duragraph)

### Option 2: Manual Deploy via Dashboard

1. Go to [Render Dashboard](https://dashboard.render.com/)
2. Click "New" → "Blueprint"
3. Connect your GitHub repository
4. Render will auto-detect `render.yaml`
5. Review the services
6. Click "Apply"

### Option 3: Deploy via CLI

```bash
# Install Render CLI
npm install -g @render/cli

# Login
render login

# Create services from blueprint
render blueprint launch
```

## Architecture

On Render, DuraGraph runs with:

- **Web Service**: DuraGraph API server (public)
- **Private Service**: NATS JetStream (internal only)
- **Web Service**: Dashboard (public)
- **Database**: Managed PostgreSQL 15

## Environment Variables

Set these in the Render Dashboard under each service's "Environment" tab:

### API Service
- `DB_*` - Auto-configured from database connection
- `NATS_URL` - Set to `nats://duragraph-nats:4222`
- `OPENAI_API_KEY` - Optional, add as secret
- `ANTHROPIC_API_KEY` - Optional, add as secret
- `JWT_SECRET` - Auto-generated, or set manually
- `AUTH_ENABLED` - Set to "true" to enable authentication

## Scaling

### Vertical Scaling
1. Go to service settings
2. Select instance type (Starter, Standard, Pro)
3. Adjust resources

### Horizontal Scaling
1. Go to service settings
2. Increase instance count (available on Standard plan and above)

## Monitoring

- **Logs**: Dashboard → Service → Logs
- **Metrics**: Dashboard → Service → Metrics
- **Health Checks**: Automatic via `/health` endpoint

## Custom Domain

1. Go to service settings
2. Click "Custom Domain"
3. Add your domain
4. Update DNS records (CNAME or A record)

## Database Management

### Connect to PostgreSQL
```bash
# Get connection info from dashboard
render postgres connect duragraph-db
```

### Backups
- Automatic daily backups on paid plans
- Manual backups: Dashboard → Database → Backups

## Cost Estimate

### Free Tier (limited)
- Web services spin down after inactivity
- Database: N/A on free tier

### Starter Plan (~$30/month)
- API: $7/month (512MB RAM)
- NATS: $7/month (512MB RAM)
- Dashboard: $7/month (512MB RAM)
- PostgreSQL: $7/month (1GB storage, 256MB RAM)

### Standard Plan (~$100/month)
- Better performance and uptime
- Horizontal scaling available

[Render Pricing](https://render.com/pricing)

## Advanced Configuration

### Persistent Disk for NATS
NATS data is stored on a 1GB persistent disk. To increase:

1. Edit `render.yaml`
2. Change `sizeGB` under disk configuration
3. Redeploy

### Environment Groups
Create an environment group for shared variables:

1. Dashboard → Environment Groups
2. Create group with common vars
3. Link to services

## Troubleshooting

### Service Won't Start
- Check build logs for errors
- Verify all required environment variables are set
- Ensure Dockerfile path is correct

### Database Connection Errors
- Verify `DB_SSLMODE=require`
- Check connection string in logs
- Ensure database is in same region

### NATS Connection Issues
- Verify private service networking
- Check NATS service logs
- Ensure `NATS_URL` uses internal service name

## Updating

Push to your GitHub branch - Render auto-deploys:

```bash
git add .
git commit -m "Update DuraGraph"
git push origin main
```

## Support

- [Render Docs](https://render.com/docs)
- [Render Community](https://community.render.com/)
