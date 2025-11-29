# Deploy DuraGraph on Railway

Deploy DuraGraph to Railway with one-click deployment and managed PostgreSQL.

## Prerequisites

- Railway account
- GitHub repository (optional, can deploy from CLI)

## Quick Deploy

### Option 1: One-Click Deploy

[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/template/duragraph)

This will create:
- DuraGraph API service
- PostgreSQL database
- NATS JetStream service
- Dashboard service

### Option 2: Manual Deploy via Dashboard

1. Go to [Railway Dashboard](https://railway.app/new)
2. Click "Deploy from GitHub repo"
3. Select your repository
4. Railway will auto-detect the configuration
5. Add PostgreSQL database: New → Database → PostgreSQL
6. Add NATS service: New → Docker Image → `nats:2.10-alpine`
7. Configure environment variables

### Option 3: Deploy via CLI

```bash
# Install Railway CLI
npm i -g @railway/cli
# or
brew install railway

# Login
railway login

# Initialize project
railway init

# Link to project
railway link

# Add PostgreSQL
railway add --database postgresql

# Deploy
railway up

# Set environment variables
railway variables set OPENAI_API_KEY=your-key-here
railway variables set ANTHROPIC_API_KEY=your-key-here
railway variables set JWT_SECRET=$(openssl rand -hex 32)
```

## Architecture

On Railway, DuraGraph runs with:

- **API Service**: DuraGraph server (public, auto-domain)
- **NATS Service**: NATS JetStream (private networking)
- **Dashboard Service**: Svelte dashboard (public, auto-domain)
- **Database**: Managed PostgreSQL 15

## Environment Variables

Railway automatically injects database credentials. Set these manually:

### API Service
```bash
# Auto-injected by Railway PostgreSQL plugin
DATABASE_URL=postgresql://...

# Or set individually (Railway provides these automatically)
DB_HOST=${{Postgres.PGHOST}}
DB_PORT=${{Postgres.PGPORT}}
DB_USER=${{Postgres.PGUSER}}
DB_PASSWORD=${{Postgres.PGPASSWORD}}
DB_NAME=${{Postgres.PGDATABASE}}
DB_SSLMODE=require

# NATS (using Railway's private networking)
NATS_URL=nats://${{NATS.RAILWAY_PRIVATE_DOMAIN}}:4222

# Server config
PORT=8080
HOST=0.0.0.0

# Optional: LLM API keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-...

# Optional: Authentication
AUTH_ENABLED=false
JWT_SECRET=your-secret-here
```

### NATS Service
Use Docker image: `nats:2.10-alpine`

Start command: `-js -sd /data -m 8222`

## Private Networking

Railway provides private networking between services. Reference other services:

```bash
# NATS URL in API service
NATS_URL=nats://${{NATS.RAILWAY_PRIVATE_DOMAIN}}:4222

# Or use service name
NATS_URL=nats://nats.railway.internal:4222
```

## Scaling

### Vertical Scaling
Railway automatically scales resources based on usage.

### Adjust Resources
```bash
railway service --settings

# Or via dashboard: Service → Settings → Resources
```

## Monitoring

### Logs
```bash
# View logs
railway logs

# Follow logs
railway logs --follow
```

### Metrics
- CPU, Memory, Network usage in Dashboard
- Custom metrics via Prometheus endpoint

## Custom Domain

```bash
# Add domain via CLI
railway domain

# Or via Dashboard: Service → Settings → Domains
```

## Database Management

### Connect to PostgreSQL
```bash
# Via Railway CLI
railway connect postgres

# Or get connection string
railway variables | grep DATABASE_URL
```

### Migrations
```bash
# Run migrations (Railway runs init scripts automatically)
railway run psql $DATABASE_URL -f deploy/sql/001_init.sql
```

## Cost Estimate

### Hobby Plan (Free $5/month credit)
- Good for testing and small projects
- Sleeps after inactivity

### Developer Plan ($20/month)
- $20 credit + usage-based pricing
- No sleep
- ~$0.000231/GB-hour for memory
- ~$0.000463/vCPU-hour

### Team Plan (Custom)
- Higher resource limits
- Priority support

[Railway Pricing](https://railway.app/pricing)

## Features

### Advantages
- ✅ Zero-config PostgreSQL with automatic connection strings
- ✅ Private networking between services
- ✅ Automatic HTTPS
- ✅ Preview environments for PRs
- ✅ One-click rollbacks
- ✅ Built-in CI/CD

### Limitations
- ⚠️ No managed NATS (must deploy as Docker service)
- ⚠️ Usage-based pricing can be unpredictable

## Deployment Workflow

```bash
# Make changes
git add .
git commit -m "Update feature"

# Deploy to production
git push origin main

# Create preview environment (for testing)
git checkout -b feature-branch
git push origin feature-branch
# Railway automatically creates preview environment
```

## Environment Setup

### Development Environment
```bash
# Create new environment
railway environment create development

# Switch environments
railway environment development

# Deploy to dev
railway up
```

### Production Environment
```bash
railway environment production
railway up
```

## Advanced Configuration

### Volume Mounts (for NATS persistence)
1. Service → Settings → Volumes
2. Mount path: `/data`
3. Size: 1GB+

### Health Checks
Railway automatically monitors `/health` endpoint.

### Build Settings
Edit `railway.json` or `railway.toml` for custom build configuration.

## Troubleshooting

### Build Failures
```bash
# Check build logs
railway logs --deployment

# Rebuild
railway up --force
```

### Service Not Starting
- Verify `PORT` environment variable
- Check logs: `railway logs`
- Ensure health check endpoint is accessible

### Database Connection Issues
- Verify SSL mode: `DB_SSLMODE=require`
- Check `DATABASE_URL` or individual DB variables
- Ensure service has access to database

### NATS Connection Issues
- Verify NATS service is running: `railway status`
- Check private networking configuration
- Ensure correct NATS URL format

## Support

- [Railway Docs](https://docs.railway.app/)
- [Railway Discord](https://discord.gg/railway)
- [Railway GitHub](https://github.com/railwayapp)

## Tips

1. **Use shared variables**: Store common config in shared variables
2. **Enable PR deployments**: Automatic preview environments for each PR
3. **Set up monitoring**: Use the Prometheus metrics endpoint
4. **Use templates**: Create Railway template for easy team deployment
