# DuraGraph Deployment Guide

Deploy DuraGraph to your preferred cloud platform with these pre-configured deployment options.

## Available Platforms

| Platform | Best For | Pricing | Setup Time | Docs |
|----------|----------|---------|------------|------|
| **Fly.io** | Global edge deployment, low latency | Pay-per-use, free tier available | 5 min | [Guide](./fly/README.md) |
| **DigitalOcean** | Simple, predictable pricing | Fixed monthly ($30+) | 10 min | [Guide](./digitalocean/README.md) |
| **Render** | Easy setup, managed services | $30/month starter | 5 min | [Guide](./render/README.md) |
| **Railway** | Developer-friendly, PR previews | $20/month + usage | 3 min | [Guide](./railway/README.md) |
| **Scaleway** | European hosting, serverless | Pay-per-use ($15+) | 10 min | [Guide](./scaleway/README.md) |
| **Pulumi (Multi-cloud)** | Advanced users, IaC, multi-cloud | Varies by provider | 15 min | [Guide](./pulumi/README.md) |

## Quick Comparison

### Fly.io
- ✅ Global edge network (low latency worldwide)
- ✅ Free tier (3 VMs included)
- ✅ Built-in PostgreSQL and NATS support
- ⚠️ Requires CLI familiarity
- 📍 Best for: Production apps with global users

### DigitalOcean App Platform
- ✅ Simple UI, great docs
- ✅ Managed PostgreSQL
- ✅ Predictable pricing
- ⚠️ Limited regions
- 📍 Best for: Startups, small businesses

### Render
- ✅ Zero-config deployments
- ✅ Automatic SSL
- ✅ Preview environments
- ⚠️ Cold starts on free tier
- 📍 Best for: Side projects, MVPs

### Railway
- ✅ Best developer experience
- ✅ One-click PostgreSQL
- ✅ PR preview environments
- ⚠️ Usage-based pricing can be unpredictable
- 📍 Best for: Developers, rapid prototyping

### Scaleway
- ✅ European data residency (GDPR)
- ✅ Serverless containers (auto-scaling)
- ✅ Pay-per-use pricing
- ⚠️ Smaller community
- 📍 Best for: EU businesses, compliance needs

### Pulumi (Multi-Cloud IaC)
- ✅ Deploy to any cloud with same codebase
- ✅ Type-safe infrastructure (TypeScript)
- ✅ Preview changes before applying
- ✅ Version control for infrastructure
- ⚠️ Requires IaC knowledge
- 📍 Best for: Teams, advanced users, multi-cloud setups

## Architecture Overview

All deployments include:

```
┌─────────────────────────────────────────────┐
│                DuraGraph Stack              │
├─────────────────────────────────────────────┤
│  API Server (Go/Echo)                       │
│  - LangGraph Cloud-compatible API           │
│  - Event sourcing with CQRS                 │
│  - GraphQL execution engine                 │
├─────────────────────────────────────────────┤
│  PostgreSQL 15                              │
│  - Event store                              │
│  - Projections (read models)                │
│  - Outbox pattern                           │
├─────────────────────────────────────────────┤
│  NATS JetStream                             │
│  - Event bus                                │
│  - Server-Sent Events (SSE)                 │
├─────────────────────────────────────────────┤
│  Dashboard (Svelte)                         │
│  - Real-time workflow visualization         │
│  - Run monitoring                           │
└─────────────────────────────────────────────┘
```

## Prerequisites

All platforms require:
- Git repository (GitHub, GitLab, or Bitbucket)
- Docker knowledge (basic)
- Environment variables configured

## Required Environment Variables

```bash
# Database (auto-configured on most platforms)
DB_HOST=your-db-host
DB_PORT=5432
DB_USER=your-db-user
DB_PASSWORD=your-db-password
DB_NAME=duragraph
DB_SSLMODE=require  # Use 'require' in production

# NATS (use internal service name)
NATS_URL=nats://nats:4222

# Server
PORT=8080
HOST=0.0.0.0

# Optional: LLM Integration
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-...

# Optional: Authentication
AUTH_ENABLED=false
JWT_SECRET=your-secret-here  # Generate with: openssl rand -hex 32
```

## Local Development

Before deploying, test locally:

```bash
# Clone repository
git clone https://github.com/Duragraph/duragraph.git
cd duragraph

# Start all services
docker-compose up -d

# Check health
curl http://localhost:8081/health

# View logs
docker-compose logs -f server
```

## Deployment Steps

### 1. Choose Your Platform
Pick a platform based on your needs (see comparison above).

### 2. Follow Platform Guide
Navigate to the platform-specific README:
- [Fly.io](./fly/README.md)
- [DigitalOcean](./digitalocean/README.md)
- [Render](./render/README.md)
- [Railway](./railway/README.md)
- [Scaleway](./scaleway/README.md)

### 3. Configure Secrets
Set up API keys and secrets via platform dashboard or CLI.

### 4. Deploy
Most platforms support:
- **One-click deploy** (from template)
- **Git-based deploy** (auto-deploy on push)
- **CLI deploy** (manual control)

### 5. Run Migrations
Database migrations run automatically on first boot (via init scripts).

### 6. Verify Deployment
```bash
# Check health
curl https://your-app-url/health

# Test API
curl https://your-app-url/api/v1/assistants

# View dashboard
open https://your-dashboard-url
```

## Post-Deployment

### Set Up Monitoring
- Enable platform-native monitoring
- Configure Prometheus scraping (endpoint: `/metrics`)
- Set up alerts for errors and downtime

### Custom Domain
All platforms support custom domains:
1. Add domain in platform dashboard
2. Update DNS records (CNAME or A)
3. SSL certificates are auto-provisioned

### Scaling
- **Vertical**: Increase CPU/RAM via platform settings
- **Horizontal**: Add more instances (most platforms support auto-scaling)

### Backups
- **Database**: Enable automated backups (daily recommended)
- **Volumes**: NATS data persistence (if applicable)

## Cost Optimization Tips

1. **Start small**: Begin with basic tier, scale as needed
2. **Use auto-scaling**: Pay only for what you use
3. **Enable caching**: Reduce database load
4. **Monitor usage**: Set up budget alerts
5. **Optimize cold starts**: Keep minimum instances warm

## Troubleshooting

### Common Issues

#### Service Won't Start
- Check build logs for errors
- Verify all environment variables are set
- Ensure Dockerfile path is correct

#### Database Connection Errors
- Verify `DB_SSLMODE=require` for production
- Check connection string format
- Ensure database is in same region

#### NATS Connection Issues
- Verify internal networking between services
- Check NATS service logs
- Use correct URL format: `nats://service-name:4222`

#### High Memory Usage
- Increase instance size
- Check for memory leaks in logs
- Optimize database queries

### Getting Help

- **Documentation**: [duragraph.dev/docs](https://duragraph.dev/docs)
- **GitHub Issues**: [github.com/Duragraph/duragraph/issues](https://github.com/Duragraph/duragraph/issues)
- **Discussions**: [github.com/Duragraph/duragraph/discussions](https://github.com/Duragraph/duragraph/discussions)

## Production Checklist

Before going to production:

- [ ] Set `DB_SSLMODE=require`
- [ ] Enable authentication (`AUTH_ENABLED=true`)
- [ ] Set strong `JWT_SECRET`
- [ ] Configure custom domain
- [ ] Enable HTTPS (auto on most platforms)
- [ ] Set up database backups
- [ ] Configure monitoring and alerts
- [ ] Enable auto-scaling
- [ ] Test disaster recovery
- [ ] Document runbook for your team

## Next Steps

1. Deploy to your chosen platform
2. Configure custom domain
3. Set up monitoring
4. Integrate with your application
5. Monitor performance and scale as needed

Happy deploying! 🚀
