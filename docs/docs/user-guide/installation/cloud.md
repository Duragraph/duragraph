# Duragraph Cloud

**Duragraph Cloud** provides a fully-managed, enterprise-ready orchestration platform with all the power of self-hosted Duragraph plus managed infrastructure, team collaboration, and enterprise security.

!!! info "Availability"
    **Status**: Coming Soon (Q2 2024)  
    **Early Access**: [Join the waitlist](https://duragraph.ai/cloud/waitlist)  
    **Features**: All self-hosted features + managed infrastructure

## Why Duragraph Cloud?

### 🚀 **Zero Infrastructure Management**
- Fully managed Temporal clusters
- Auto-scaling workers and API servers
- Managed PostgreSQL with automated backups
- 99.9% uptime SLA

### 👥 **Team Collaboration**
- Multi-user workspaces
- Role-based access control (RBAC)
- Shared workflows and assistants
- Team activity dashboards

### 🔒 **Enterprise Security**
- SOC 2 Type II compliance
- End-to-end encryption
- VPC peering and private networking
- Audit logs and compliance reporting

### 📊 **Advanced Observability**
- Real-time performance analytics
- Custom metrics and alerting
- Distributed tracing
- Cost optimization insights

## Getting Started

### 1. Sign Up

```bash
# Join the waitlist (available Q2 2024)
curl -X POST https://api.duragraph.ai/waitlist \
  -H "Content-Type: application/json" \
  -d '{"email": "you@company.com", "company": "Your Company"}'
```

Or visit: [https://duragraph.ai/cloud/signup](https://duragraph.ai/cloud/signup)

### 2. Create Your First Workspace

```bash
# Install Duragraph CLI
npm install -g @duragraph/cli

# Login to Duragraph Cloud
duragraph auth login

# Create workspace
duragraph workspace create "My Company Workspace"

# Set as default
duragraph workspace use "My Company Workspace"
```

### 3. Deploy Your First Workflow

```python
from duragraph import DuragraphClient

# Connect to Duragraph Cloud
client = DuragraphClient(
    base_url="https://api.duragraph.ai",
    api_key="dg_live_...",  # From dashboard
    workspace_id="ws_..."
)

# Create and run workflow
workflow = client.create_workflow({
    "name": "Customer Support Agent",
    "steps": [
        {
            "type": "llm_call",
            "model": "gpt-4",
            "messages": [
                {"role": "system", "content": "You are a helpful customer support agent."},
                {"role": "user", "content": "{customer_query}"}
            ]
        }
    ]
})

# Execute
run = client.create_run(
    workflow_id=workflow.id,
    inputs={"customer_query": "How do I reset my password?"}
)

print(f"Run started: {run.id}")
```

## Features

### **Managed Infrastructure**

**Auto-scaling Workers**
- Python, Go, and TypeScript workers
- Automatic scaling based on queue depth
- Resource optimization and cost control

**Managed Temporal**
- High-availability Temporal clusters
- Automatic upgrades and maintenance
- Custom namespace isolation

**Database Management**
- Managed PostgreSQL with automated backups
- Point-in-time recovery
- Read replicas for high-query workloads

### **Team Management**

**Workspaces**
- Isolated environments for teams/projects
- Resource quotas and billing controls
- Cross-workspace collaboration

**Role-Based Access Control**
```bash
# Add team member
duragraph team add user@company.com --role=developer

# Create custom role
duragraph role create "workflow-admin" \
  --permissions="workflows:create,workflows:read,workflows:update"

# Assign role to user
duragraph user assign user@company.com --role="workflow-admin"
```

**Permissions Model**
- `workspace:admin` - Full workspace control
- `workflow:developer` - Create/edit workflows  
- `workflow:viewer` - Read-only access
- `runtime:operator` - Manage runs and executions

### **Enterprise Security**

**Authentication**
- SSO with SAML/OIDC
- Multi-factor authentication (MFA)
- API key management

**Network Security**
- VPC peering for private connectivity
- IP allowlisting
- mTLS for service-to-service communication

**Data Protection**
- Encryption at rest (AES-256)
- Encryption in transit (TLS 1.3)
- Key management via AWS KMS/Azure Key Vault

### **Advanced Observability**

**Real-time Dashboards**
- Workflow execution metrics
- Resource utilization tracking
- Error rates and latency monitoring

**Custom Alerting**
```yaml
# duragraph-alerts.yml
alerts:
  - name: "High Error Rate"
    condition: "error_rate > 5%"
    channels: ["slack", "pagerduty"]
    
  - name: "Long Running Workflow"
    condition: "workflow_duration > 30m"
    channels: ["email"]
```

**Distributed Tracing**
- OpenTelemetry integration
- Cross-service trace correlation
- Performance bottleneck identification

## Pricing

### **Developer Plan** - Free
- 1 workspace
- 10,000 workflow executions/month
- Community support
- Basic observability

### **Team Plan** - $99/month
- 5 workspaces
- 100,000 executions/month
- Team collaboration features
- Email support
- Advanced observability

### **Enterprise Plan** - Custom
- Unlimited workspaces
- Custom execution limits
- SSO and advanced security
- 24/7 support with SLA
- Dedicated infrastructure options

## Migration from Self-Hosted

### **Export from Self-Hosted**

```bash
# Export workflows
duragraph export workflows --output=workflows.json

# Export assistants
duragraph export assistants --output=assistants.json

# Export historical runs (optional)
duragraph export runs --date-range="2024-01-01,2024-03-01"
```

### **Import to Cloud**

```bash
# Login to cloud
duragraph auth login --cloud

# Import workflows
duragraph import workflows workflows.json

# Import assistants
duragraph import assistants assistants.json

# Verify migration
duragraph workflow list
```

### **Gradual Migration**

```python
# Use feature flags for gradual migration
import os

if os.getenv("USE_DURAGRAPH_CLOUD") == "true":
    client = DuragraphClient(
        base_url="https://api.duragraph.ai",
        api_key=os.getenv("DURAGRAPH_CLOUD_API_KEY")
    )
else:
    client = DuragraphClient(
        base_url="http://localhost:8080"
    )
```

## Support

### **Documentation**
- [Cloud API Reference](https://docs.duragraph.ai/cloud/api)
- [Team Management Guide](https://docs.duragraph.ai/cloud/teams)
- [Security Best Practices](https://docs.duragraph.ai/cloud/security)

### **Support Channels**
- **Community**: [GitHub Discussions](https://github.com/adwiteeymauriya/duragraph/discussions)
- **Email**: support@duragraph.ai
- **Enterprise**: 24/7 support with SLA guarantees

### **Status Page**
Monitor Duragraph Cloud status: [https://status.duragraph.ai](https://status.duragraph.ai)

## Enterprise Features

### **Compliance**
- SOC 2 Type II certification
- GDPR compliance
- HIPAA eligibility (Business Associate Agreement)
- ISO 27001 alignment

### **Advanced Deployment**
- Private cloud deployments
- Customer-managed encryption keys
- Custom retention policies
- Disaster recovery options

### **Professional Services**
- Migration assistance
- Custom training programs
- Architecture review and optimization
- 24/7 dedicated support

## Early Access

**Join the Waitlist**: [https://duragraph.ai/cloud/waitlist](https://duragraph.ai/cloud/waitlist)

**Beta Program**:
- Priority access to new features
- Direct feedback channel to engineering
- No usage limits during beta
- Migration assistance included

**Timeline**:
- **Q1 2024**: Private beta with select partners
- **Q2 2024**: Public beta launch
- **Q3 2024**: General availability

## Next Steps

🔗 [Join the waitlist](https://duragraph.ai/cloud/waitlist)  
📚 [Compare with self-hosted](self-hosted.md)  
🛠️ [Start with local development](../quickstart.md)  
💬 [Join our community](https://github.com/adwiteeymauriya/duragraph/discussions)
