# Deploy DuraGraph on Scaleway

Deploy DuraGraph to Scaleway's Serverless Containers with managed PostgreSQL.

## Prerequisites

- [Scaleway account](https://console.scaleway.com/register)
- [Scaleway CLI (scw)](https://github.com/scaleway/scaleway-cli) installed

## Quick Deploy

### Option 1: Deploy via Console

1. Go to [Scaleway Console](https://console.scaleway.com/)
2. Navigate to Serverless → Containers
3. Click "Deploy a container"
4. Select "From source code" and connect your GitHub repo
5. Scaleway will auto-detect `scaleway.yaml`
6. Configure secrets
7. Click "Deploy"

### Option 2: Deploy via CLI

```bash
# Install Scaleway CLI
brew install scw  # macOS
# or
curl -o /usr/local/bin/scw -L https://github.com/scaleway/scaleway-cli/releases/latest/download/scw-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)
chmod +x /usr/local/bin/scw

# Initialize CLI
scw init

# Create PostgreSQL database
scw rdb instance create \
  name=duragraph-db \
  engine=PostgreSQL-15 \
  node-type=db-dev-s \
  volume-size=10

# Get database credentials
DB_INSTANCE_ID=$(scw rdb instance list name=duragraph-db -o json | jq -r '.[0].id')
scw rdb instance get $DB_INSTANCE_ID

# Deploy containers
scw container deploy \
  --name duragraph-api \
  --dockerfile-path deploy/docker/Dockerfile.server \
  --port 8080

# Set secrets
scw container secret create \
  name=OPENAI_API_KEY \
  value=your-key-here

scw container secret create \
  name=ANTHROPIC_API_KEY \
  value=your-key-here

scw container secret create \
  name=JWT_SECRET \
  value=$(openssl rand -hex 32)
```

## Architecture

On Scaleway, DuraGraph runs with:

- **Serverless Containers**: DuraGraph API (auto-scaling)
- **Serverless Containers**: NATS JetStream (private)
- **Serverless Containers**: Dashboard (public)
- **Managed Database**: PostgreSQL 15

## Container Configuration

### API Container
- **Min instances**: 1
- **Max instances**: 5 (auto-scales based on load)
- **CPU**: 1 vCPU
- **Memory**: 1GB
- **Port**: 8080
- **Privacy**: Public (with HTTPS)

### NATS Container
- **Instances**: 1 (fixed)
- **CPU**: 0.5 vCPU
- **Memory**: 512MB
- **Port**: 4222 (internal)
- **Privacy**: Private
- **Storage**: 5GB SSD volume

### Dashboard Container
- **Min instances**: 1
- **Max instances**: 3
- **CPU**: 0.5 vCPU
- **Memory**: 512MB
- **Port**: 80

## Environment Variables

### Set via Console
1. Container → Settings → Environment Variables
2. Add secrets via Secrets Manager

### Set via CLI
```bash
# Regular environment variables
scw container update <container-id> \
  env.PORT=8080 \
  env.HOST=0.0.0.0

# Secrets (encrypted)
scw container secret create \
  container-id=<container-id> \
  name=OPENAI_API_KEY \
  value=your-key
```

### Required Variables
- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` - From managed database
- `NATS_URL` - Set to `nats://nats:4222` (private networking)
- `JWT_SECRET` - For authentication (if enabled)

### Optional Variables
- `OPENAI_API_KEY` - OpenAI integration
- `ANTHROPIC_API_KEY` - Anthropic integration
- `AUTH_ENABLED` - Set to "true" to enable JWT auth

## Scaling

### Auto-scaling Configuration
```bash
# Update min/max instances
scw container update <container-id> \
  min-scale=1 \
  max-scale=10

# Adjust resources
scw container update <container-id> \
  cpu-limit=2000 \
  memory-limit=2048
```

### Scaling Triggers
- **Concurrent requests**: Scales up when requests > CPU limit
- **CPU usage**: Scales up at 70% CPU
- **Memory usage**: Scales up at 80% memory

## Monitoring

### Logs
```bash
# View logs via CLI
scw container logs <container-id>

# Follow logs
scw container logs <container-id> --follow

# Via Console: Container → Logs
```

### Metrics
- **Console**: Container → Metrics
  - Request count
  - Response time
  - Error rate
  - CPU/Memory usage
- **Cockpit**: Scaleway's observability platform
  - Custom dashboards
  - Alerts
  - Log aggregation

### Set up Alerts
```bash
scw cockpit alert create \
  --metric container.cpu.usage \
  --threshold 80 \
  --email your@email.com
```

## Database Management

### Connect to PostgreSQL
```bash
# Get connection info
scw rdb instance get <instance-id>

# Connect via psql
psql "postgresql://user:password@host:port/database?sslmode=require"
```

### Backups
- **Automatic**: Daily backups at 2 AM (configurable)
- **Manual**: Console → Database → Backups → Create backup
- **Restore**: Console → Database → Backups → Restore

### Migrations
```bash
# Run migrations
psql $DATABASE_URL < deploy/sql/001_init.sql
psql $DATABASE_URL < deploy/sql/002_event_store.sql
psql $DATABASE_URL < deploy/sql/003_outbox.sql
psql $DATABASE_URL < deploy/sql/004_projections.sql
```

## Custom Domain

### Via Console
1. Container → Settings → Domains
2. Add custom domain
3. Create CNAME record: `your-domain.com` → `<container-url>.scw.cloud`

### Via CLI
```bash
scw container domain create \
  container-id=<container-id> \
  hostname=api.yourdomain.com
```

## Cost Estimate

### Serverless Containers (Pay-per-use)
- **Compute**: €0.000010/vCPU-second + €0.000001/MB-second
- **Requests**: €0.40/million requests
- **Free tier**: 400,000 vCPU-seconds + 200,000 GB-seconds per month

### Managed PostgreSQL
- **Dev instance (db-dev-s)**: ~€10/month
  - 2 vCPU, 2GB RAM, 10GB storage
- **Standard instance (db-gp-s)**: ~€50/month
  - 4 vCPU, 16GB RAM, 100GB storage

### Example Monthly Cost
- **Light usage** (~100k requests): ~€15/month (API + NATS + DB)
- **Medium usage** (~1M requests): ~€25/month
- **Heavy usage** (~10M requests): ~€60/month

[Scaleway Pricing](https://www.scaleway.com/en/pricing/)

## Networking

### Private Networking
Containers in the same namespace can communicate via private network:
- Use service names: `nats:4222`, `postgres:5432`
- No internet routing (faster, secure)

### Public Endpoints
- API: `https://<container-name>-<namespace>.scw.cloud`
- Dashboard: `https://<dashboard-name>-<namespace>.scw.cloud`

## CI/CD Integration

### GitHub Actions
```yaml
name: Deploy to Scaleway
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Deploy
        env:
          SCW_SECRET_KEY: ${{ secrets.SCW_SECRET_KEY }}
        run: |
          scw container deploy --name duragraph-api
```

### GitLab CI
```yaml
deploy:
  image: scaleway/cli
  script:
    - scw container deploy --name duragraph-api
  only:
    - main
```

## Advanced Features

### Cold Start Optimization
```bash
# Keep minimum instances warm
scw container update <container-id> --min-scale=1

# Or use "Scale to Zero" with faster cold starts
scw container update <container-id> --min-scale=0 --timeout=300s
```

### Volumes for NATS Persistence
- Local SSD volumes (faster, ephemeral)
- Block Storage volumes (persistent, slower)

### VPC Integration
```bash
# Create private network
scw vpc private-network create name=duragraph-net

# Attach containers
scw container update <container-id> \
  private-network-id=<network-id>
```

## Troubleshooting

### Container Won't Start
- Check logs: `scw container logs <container-id>`
- Verify environment variables
- Check health check endpoint

### Database Connection Errors
- Ensure `DB_SSLMODE=require`
- Verify database instance is running
- Check credentials

### High Latency
- Enable multi-AZ deployment for database
- Use Scaleway CDN for static assets
- Optimize container placement (same region)

### NATS Connection Issues
- Verify private networking is enabled
- Check NATS container logs
- Ensure correct URL format

## Regions

Scaleway is available in:
- **Paris (PAR)** - fr-par-1, fr-par-2, fr-par-3
- **Amsterdam (AMS)** - nl-ams-1, nl-ams-2
- **Warsaw (WAW)** - pl-waw-1, pl-waw-2

Choose region closest to your users for best performance.

## Support

- [Scaleway Documentation](https://www.scaleway.com/en/docs/)
- [Community Slack](https://slack.scaleway.com/)
- [Support Tickets](https://console.scaleway.com/support/tickets)

## Best Practices

1. **Use secrets manager** for sensitive data
2. **Enable auto-scaling** for cost optimization
3. **Set up monitoring** and alerts
4. **Use private networking** between services
5. **Enable database backups**
6. **Use custom domains** for production
7. **Implement health checks** for reliability
