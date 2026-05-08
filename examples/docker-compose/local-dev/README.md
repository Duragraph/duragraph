# Local Development Stack

Complete local development environment with DuraGraph and all dependencies.

## Services

| Service | Port | Description |
|---------|------|-------------|
| DuraGraph API | 8081 | Control plane REST API |
| DuraGraph Studio | 3000 | Interactive UI for agent interaction |
| PostgreSQL | 5432 | Event store and projections |
| NATS JetStream | 4222 | Message broker |
| NATS Monitoring | 8222 | NATS HTTP monitoring |
| Prometheus Metrics | 9090 | DuraGraph metrics |

## Quick Start

1. **Start all services:**
   ```bash
   docker compose up -d
   ```

2. **Check health:**
   ```bash
   curl http://localhost:8081/health
   ```

3. **View logs:**
   ```bash
   docker compose logs -f duragraph
   ```

4. **Stop services:**
   ```bash
   docker compose down
   ```

## With Redis (Optional)

For caching support, start with the Redis profile:

```bash
docker compose --profile with-redis up -d
```

## Accessing Services

- **DuraGraph Studio:** http://localhost:3000 (Interactive UI)
- **DuraGraph API:** http://localhost:8081
- **Health Check:** http://localhost:8081/health
- **Metrics:** http://localhost:9090/metrics
- **NATS Monitor:** http://localhost:8222

## Testing the Setup

Create a test assistant:

```bash
curl -X POST http://localhost:8081/api/v1/assistants \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Assistant",
    "graph_id": "hello-world"
  }'
```

## Environment Variables

Copy `.env.example` to `.env` to customize:

```bash
cp .env.example .env
```

| Variable | Default | Description |
|----------|---------|-------------|
| `DURAGRAPH_PORT` | 8081 | API server port |
| `POSTGRES_USER` | duragraph | Database user |
| `POSTGRES_PASSWORD` | duragraph | Database password |

## Troubleshooting

### DuraGraph not starting

Check if PostgreSQL is ready:
```bash
docker compose logs postgres
```

### NATS connection issues

Verify NATS is running:
```bash
curl http://localhost:8222/varz
```

### Reset everything

```bash
docker compose down -v  # Removes volumes
docker compose up -d
```

## Next Steps

1. Run an example worker: [python/01-hello-world](../../python/01-hello-world)
2. Create your first graph
3. Monitor with the dashboard
