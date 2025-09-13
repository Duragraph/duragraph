# Self-Hosted Installation

Deploy Duragraph in your own infrastructure for full control, data sovereignty, and cost optimization.

## Quick Start (Docker Compose)

The fastest way to get Duragraph running locally or on a single server:

```bash
# Clone the repository
git clone https://github.com/adwiteeymauriya/duragraph.git
cd duragraph

# Start all services
docker compose up -d

# Verify services are running
docker compose ps
```

This starts:
- **API Server** (`:8080`) - REST API and SSE streaming
- **Temporal Server** (`:7233`) - Workflow orchestration  
- **PostgreSQL** (`:5432`) - Database for state/checkpoints
- **Temporal Web UI** (`:8088`) - Temporal dashboard
- **Duragraph Dashboard** (`:3000`) - Web interface

## Production Deployment

### Prerequisites

- Docker and Docker Compose (or Kubernetes)
- PostgreSQL 13+ (managed or self-hosted)
- 4+ CPU cores, 8GB+ RAM (minimum)
- SSL certificate for HTTPS

### Environment Configuration

Create a `.env` file:

```bash
# Database
DATABASE_URL=postgresql://user:password@db-host:5432/duragraph
POSTGRES_USER=duragraph
POSTGRES_PASSWORD=your-secure-password
POSTGRES_DB=duragraph

# Temporal
TEMPORAL_ADDRESS=temporal:7233
TEMPORAL_NAMESPACE=duragraph

# API Configuration  
API_PORT=8080
API_HOST=0.0.0.0
LOG_LEVEL=info

# Security
JWT_SECRET=your-jwt-secret-key
API_KEY=your-api-key

# Optional: External services
REDIS_URL=redis://redis:6379
S3_BUCKET=duragraph-checkpoints
S3_REGION=us-west-2
```

### Docker Compose (Recommended)

Use the production-ready compose file:

```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  api:
    image: duragraph/api:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=${DATABASE_URL}
      - TEMPORAL_ADDRESS=${TEMPORAL_ADDRESS}
      - JWT_SECRET=${JWT_SECRET}
    depends_on:
      - postgres
      - temporal
    restart: unless-stopped
    
  temporal:
    image: temporalio/auto-setup:latest
    ports:
      - "7233:7233"
    environment:
      - DB=postgresql
      - DB_PORT=5432
      - POSTGRES_USER=${POSTGRES_USER}
      - POSTGRES_PWD=${POSTGRES_PASSWORD}
      - POSTGRES_SEEDS=postgres
    depends_on:
      - postgres
    restart: unless-stopped

  postgres:
    image: postgres:15
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=${POSTGRES_USER}
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
      - POSTGRES_DB=${POSTGRES_DB}
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./deploy/sql/:/docker-entrypoint-initdb.d/
    restart: unless-stopped

  python-worker:
    image: duragraph/python-worker:latest
    environment:
      - TEMPORAL_ADDRESS=${TEMPORAL_ADDRESS}
      - DATABASE_URL=${DATABASE_URL}
    depends_on:
      - temporal
    restart: unless-stopped
    scale: 3

  go-worker:
    image: duragraph/go-worker:latest  
    environment:
      - TEMPORAL_ADDRESS=${TEMPORAL_ADDRESS}
      - DATABASE_URL=${DATABASE_URL}
    depends_on:
      - temporal
    restart: unless-stopped
    scale: 2

volumes:
  postgres_data:
```

Start production deployment:

```bash
docker compose -f docker-compose.prod.yml up -d
```

### Kubernetes Deployment

For Kubernetes deployments, use our Helm chart:

```bash
# Add Duragraph Helm repository
helm repo add duragraph https://charts.duragraph.ai
helm repo update

# Install with custom values
helm install duragraph duragraph/duragraph \
  --set postgresql.auth.password=your-password \
  --set api.config.jwtSecret=your-jwt-secret \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=duragraph.yourcompany.com
```

Example `values.yaml`:

```yaml
# values.yaml
api:
  replicaCount: 3
  config:
    logLevel: info
    jwtSecret: "your-jwt-secret"
  
temporal:
    enabled: true
    replicaCount: 1
    
postgresql:
  enabled: true
  auth:
    username: duragraph
    password: "your-secure-password"
    database: duragraph
  primary:
    persistence:
      size: 100Gi

workers:
  python:
    replicaCount: 5
    resources:
      limits:
        cpu: 1000m
        memory: 2Gi
  go:
    replicaCount: 3
    resources:
      limits:
        cpu: 500m
        memory: 1Gi

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: duragraph.yourcompany.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: duragraph-tls
      hosts:
        - duragraph.yourcompany.com
```

## Database Setup

### PostgreSQL Schema

Initialize the database schema:

```bash
# Run migrations
docker exec duragraph-api-1 /app/migrate up

# Or manually with psql
psql $DATABASE_URL -f deploy/sql/0001_init.sql
```

### External Database

To use an external PostgreSQL instance:

```bash
# Set database URL
export DATABASE_URL="postgresql://user:pass@your-db-host:5432/duragraph"

# Run only API and workers (no local postgres)
docker compose up api temporal python-worker go-worker
```

## Scaling

### Horizontal Scaling

Scale workers based on load:

```bash
# Scale Python workers
docker compose up --scale python-worker=5

# Scale Go workers  
docker compose up --scale go-worker=3

# Scale API servers (behind load balancer)
docker compose up --scale api=3
```

### Vertical Scaling

Adjust resource limits:

```yaml
# docker-compose.override.yml
services:
  python-worker:
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 4G
        reservations:
          cpus: '1.0'
          memory: 2G
```

## Security

### SSL/TLS Setup

Use a reverse proxy like nginx:

```nginx
# /etc/nginx/sites-available/duragraph
server {
    listen 443 ssl http2;
    server_name duragraph.yourcompany.com;
    
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # SSE support
        proxy_buffering off;
        proxy_cache off;
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
    }
}
```

### Authentication

Configure authentication methods:

```bash
# JWT-based auth
export JWT_SECRET="your-256-bit-secret"

# API key auth  
export API_KEY="your-api-key"

# OAuth integration (coming soon)
export OAUTH_PROVIDER="google"
export OAUTH_CLIENT_ID="your-client-id"
```

## Monitoring

### Health Checks

Built-in health endpoints:

```bash
# API health
curl http://localhost:8080/health

# Temporal health
curl http://localhost:8088/api/v1/health

# Database health
curl http://localhost:8080/health/db
```

### Metrics

Duragraph exposes Prometheus metrics:

```bash
# Scrape metrics
curl http://localhost:8080/metrics
```

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'duragraph'
    static_configs:
      - targets: ['duragraph-api:8080']
    metrics_path: /metrics
```

### Logging

Configure structured logging:

```bash
# JSON logs for production
export LOG_FORMAT=json
export LOG_LEVEL=info

# View logs
docker compose logs -f api
docker compose logs -f python-worker
```

## Backup and Recovery

### Database Backups

```bash
# Automated backups
docker exec postgres pg_dump -U duragraph duragraph > backup.sql

# Restore
docker exec -i postgres psql -U duragraph duragraph < backup.sql
```

### Temporal Data

```bash
# Export workflows
tctl workflow list --namespace duragraph

# Backup temporal data
kubectl exec temporal-pod -- temporal workflow export
```

## Troubleshooting

### Common Issues

**Services won't start:**
```bash
# Check logs
docker compose logs api
docker compose logs temporal

# Verify database connection
docker exec api ping postgres
```

**Workers not processing:**
```bash
# Check worker logs
docker compose logs python-worker

# Verify Temporal connection
docker exec python-worker temporal health
```

**High latency:**
```bash
# Check resource usage
docker stats

# Scale workers
docker compose up --scale python-worker=10
```

### Performance Tuning

```yaml
# Optimize for high throughput
services:
  api:
    environment:
      - GO_MAX_PROCS=4
      - GC_PERCENT=100
      
  python-worker:
    environment:
      - PYTHON_WORKERS=4
      - MAX_CONCURRENT_ACTIVITIES=10
```

## Next Steps

✅ [Configure monitoring](../concepts/observability.md)  
✅ [Set up SSL certificates](#ssltls-setup)  
✅ [Scale for production](#scaling)  
✅ [Configure backups](#backup-and-recovery)  
✅ [Explore the dashboard](http://localhost:3000)

Need help? Check our [troubleshooting guide](../../operations/troubleshooting.md) or [open an issue](https://github.com/adwiteeymauriya/duragraph/issues).
