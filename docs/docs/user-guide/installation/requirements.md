# System Requirements

## Minimum Requirements

### For Development
- **CPU**: 2 cores
- **RAM**: 4GB
- **Storage**: 10GB free space
- **OS**: Linux, macOS, or Windows with WSL2

### For Production
- **CPU**: 4+ cores
- **RAM**: 8GB+ 
- **Storage**: 50GB+ free space
- **OS**: Linux (Ubuntu 20.04+, CentOS 8+, or equivalent)

## Software Dependencies

### Required
- Docker 20.10+
- Docker Compose 2.0+
- PostgreSQL 13+ (if using external database)

### Optional
- Kubernetes 1.20+ (for K8s deployments)
- Helm 3.8+ (for Helm chart deployments)
- Redis 6.0+ (for caching and job queues)

## Network Requirements

### Ports
- `8080` - API Server
- `7233` - Temporal Server
- `5432` - PostgreSQL
- `3000` - Dashboard (optional)

### Firewall
- Allow inbound traffic on required ports
- Allow outbound HTTPS (443) for LLM API calls
- Allow outbound traffic to external services (if used)

---

📚 **Next**: [Self-Hosted Installation](self-hosted.md) | [Cloud Installation](cloud.md)
