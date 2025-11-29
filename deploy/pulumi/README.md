# DuraGraph Pulumi Deployment

Deploy DuraGraph to multiple cloud providers using a unified Infrastructure as Code approach with Pulumi.

## 🎯 Why Pulumi?

- **Multi-Cloud**: Deploy to AWS, DigitalOcean, GCP, or Azure with the same codebase
- **Type Safety**: Catch infrastructure errors before deployment with TypeScript
- **Code Reuse**: Share infrastructure components across projects
- **Testing**: Unit test your infrastructure code
- **Version Control**: Track infrastructure changes in Git
- **Preview Changes**: See what will change before applying

## 📋 Prerequisites

### Required
- [Node.js](https://nodejs.org/) 18+ and npm
- [Pulumi CLI](https://www.pulumi.com/docs/get-started/install/)
- Cloud provider account (AWS, DigitalOcean, etc.)
- Cloud provider CLI configured (optional but recommended)

### Optional
- Docker (for building custom images)
- Git (for version control)

## 🚀 Quick Start

### 1. Install Pulumi

```bash
# macOS
brew install pulumi/tap/pulumi

# Linux
curl -fsSL https://get.pulumi.com | sh

# Windows
choco install pulumi

# Verify installation
pulumi version
```

### 2. Login to Pulumi

```bash
# Use Pulumi Cloud (free for individuals)
pulumi login

# Or use local backend (no account needed)
pulumi login --local
```

### 3. Install Dependencies

```bash
cd deploy/pulumi
npm install
```

### 4. Initialize Stack

```bash
# Create a new stack (e.g., dev, staging, production)
pulumi stack init dev

# Or select existing stack
pulumi stack select dev
```

### 5. Configure Provider

```bash
# Set cloud provider (aws, digitalocean, gcp, azure)
pulumi config set duragraph:provider aws

# Set region
pulumi config set duragraph:region us-east-1

# Set environment
pulumi config set duragraph:environment dev
```

### 6. Set Secrets

```bash
# JWT secret (generated if not set)
pulumi config set --secret duragraph:jwtSecret $(openssl rand -hex 32)

# Optional: LLM API keys
pulumi config set --secret duragraph:openaiApiKey sk-...
pulumi config set --secret duragraph:anthropicApiKey sk-...

# Enable authentication (optional)
pulumi config set duragraph:authEnabled true
```

### 7. Deploy

```bash
# Preview changes
pulumi preview

# Deploy infrastructure
pulumi up

# Select 'yes' when prompted
```

### 8. Get Outputs

```bash
# View all outputs
pulumi stack output

# Get specific output
pulumi stack output apiUrl
pulumi stack output dashboardUrl
```

## 🎛️ Configuration Reference

### Provider Selection

```bash
# AWS (default)
pulumi config set duragraph:provider aws
pulumi config set duragraph:region us-east-1

# DigitalOcean
pulumi config set duragraph:provider digitalocean
pulumi config set duragraph:region nyc3

# GCP (coming soon)
pulumi config set duragraph:provider gcp
pulumi config set duragraph:region us-central1

# Azure (coming soon)
pulumi config set duragraph:provider azure
pulumi config set duragraph:region eastus
```

### Scaling Configuration

```bash
# Minimum API instances
pulumi config set duragraph:apiInstanceCount 2

# Maximum API instances (auto-scaling)
pulumi config set duragraph:apiMaxInstances 10

# Database size (small, medium, large)
pulumi config set duragraph:dbInstanceSize medium
```

### Environment Variables

```bash
# Environment name
pulumi config set duragraph:environment production

# Authentication
pulumi config set duragraph:authEnabled true
pulumi config set --secret duragraph:jwtSecret your-secret

# LLM API Keys
pulumi config set --secret duragraph:openaiApiKey sk-...
pulumi config set --secret duragraph:anthropicApiKey sk-...
```

## 📊 Provider-Specific Details

### AWS Deployment

**What gets created:**
- VPC with public and private subnets
- RDS PostgreSQL 15 instance
- ECS Fargate cluster
- Application Load Balancer
- ECS services for API and NATS
- S3 + CloudFront for dashboard
- Security groups and IAM roles

**Prerequisites:**
```bash
# Install AWS CLI
brew install awscli  # macOS
# or: pip install awscli

# Configure credentials
aws configure

# Or use environment variables
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=us-east-1
```

**Cost estimate:** ~$50-150/month depending on usage

### DigitalOcean Deployment

**What gets created:**
- Managed PostgreSQL cluster
- App Platform services (API, NATS, Dashboard)
- Auto-scaling enabled
- Automatic SSL certificates

**Prerequisites:**
```bash
# Get DigitalOcean API token from: https://cloud.digitalocean.com/account/api/tokens

# Set token
export DIGITALOCEAN_TOKEN=dop_v1_...

# Or use Pulumi config
pulumi config set digitalocean:token --secret dop_v1_...
```

**Cost estimate:** ~$30-60/month depending on usage

### GCP Deployment (Coming Soon)

Will create:
- Cloud SQL PostgreSQL instance
- Cloud Run services
- Cloud Storage + CDN
- VPC networking

### Azure Deployment (Coming Soon)

Will create:
- Azure Database for PostgreSQL
- Container Instances
- Application Gateway
- Storage Account + CDN

## 🔧 Advanced Usage

### Multiple Environments

```bash
# Create separate stacks for each environment
pulumi stack init dev
pulumi config set duragraph:environment dev
pulumi config set duragraph:dbInstanceSize small

pulumi stack init staging
pulumi config set duragraph:environment staging
pulumi config set duragraph:dbInstanceSize medium

pulumi stack init production
pulumi config set duragraph:environment production
pulumi config set duragraph:dbInstanceSize large
pulumi config set duragraph:apiInstanceCount 3
```

### Custom Docker Images

Update the provider code to use your custom image:

```typescript
// In src/providers/aws.ts or digitalocean.ts
image: "your-registry/duragraph-api:latest"
```

### Stack References (Shared Resources)

Share resources across stacks:

```typescript
import * as pulumi from "@pulumi/pulumi";

// Reference another stack's outputs
const sharedStack = new pulumi.StackReference("organization/project/shared");
const vpcId = sharedStack.getOutput("vpcId");
```

### CI/CD Integration

#### GitHub Actions

```yaml
name: Deploy with Pulumi
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - uses: actions/setup-node@v3
        with:
          node-version: 18

      - name: Install dependencies
        working-directory: deploy/pulumi
        run: npm install

      - uses: pulumi/actions@v4
        with:
          command: up
          stack-name: production
          work-dir: deploy/pulumi
        env:
          PULUMI_ACCESS_TOKEN: ${{ secrets.PULUMI_ACCESS_TOKEN }}
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
```

#### GitLab CI

```yaml
deploy:
  image: pulumi/pulumi-nodejs
  script:
    - cd deploy/pulumi
    - npm install
    - pulumi login
    - pulumi stack select production
    - pulumi up --yes
  only:
    - main
  variables:
    PULUMI_ACCESS_TOKEN: $PULUMI_ACCESS_TOKEN
    AWS_ACCESS_KEY_ID: $AWS_ACCESS_KEY_ID
    AWS_SECRET_ACCESS_KEY: $AWS_SECRET_ACCESS_KEY
```

## 📈 Monitoring and Management

### View Stack Outputs

```bash
# All outputs
pulumi stack output

# JSON format
pulumi stack output --json

# Specific output
pulumi stack output apiUrl
```

### View Resources

```bash
# List all resources in stack
pulumi stack

# Show detailed resource information
pulumi stack export
```

### Update Configuration

```bash
# Update config value
pulumi config set duragraph:apiInstanceCount 5

# Apply changes
pulumi up
```

### Destroy Infrastructure

```bash
# Preview what will be destroyed
pulumi destroy --preview

# Destroy all resources
pulumi destroy

# Skip confirmation
pulumi destroy --yes
```

## 🛠️ Troubleshooting

### Stack Already Exists

```bash
# List all stacks
pulumi stack ls

# Delete existing stack
pulumi stack rm dev

# Or select and use existing
pulumi stack select dev
```

### Configuration Errors

```bash
# View all config
pulumi config

# Check for missing values
pulumi config get duragraph:provider

# Reset config value
pulumi config rm duragraph:someKey
```

### Deployment Failures

```bash
# View detailed logs
pulumi up --logtostderr -v=9

# Refresh state
pulumi refresh

# Re-run deployment
pulumi up
```

### State Conflicts

```bash
# If state is out of sync
pulumi refresh

# Export state for backup
pulumi stack export --file backup.json

# Import state if needed
pulumi stack import --file backup.json
```

### Provider Authentication Issues

```bash
# AWS
aws sts get-caller-identity

# DigitalOcean
doctl auth list

# Verify Pulumi can access credentials
pulumi config get digitalocean:token
```

## 🔐 Security Best Practices

1. **Use Secrets**: Always use `--secret` flag for sensitive data
   ```bash
   pulumi config set --secret duragraph:jwtSecret $(openssl rand -hex 32)
   ```

2. **Enable SSL**: Set `DB_SSLMODE=require` in production

3. **Use IAM Roles**: On AWS, use IAM roles instead of access keys when possible

4. **Restrict Access**: Use security groups and network ACLs

5. **Enable Backups**: Ensure database backups are enabled in production

6. **Audit Logs**: Enable CloudTrail (AWS) or equivalent on other providers

## 📚 Additional Resources

- [Pulumi Documentation](https://www.pulumi.com/docs/)
- [Pulumi AWS Guide](https://www.pulumi.com/docs/clouds/aws/get-started/)
- [Pulumi DigitalOcean Guide](https://www.pulumi.com/docs/clouds/digitalocean/get-started/)
- [DuraGraph Documentation](https://duragraph.dev/docs)

## 🤝 Contributing

To add support for new providers:

1. Create new provider file: `src/providers/yourprovider.ts`
2. Implement `CloudProvider` interface
3. Add provider case in `index.ts`
4. Update documentation
5. Submit pull request

## 💡 Tips

- Use `pulumi preview` before every deployment
- Keep stacks separate for dev/staging/production
- Use stack references for shared resources
- Enable Pulumi Cloud for team collaboration
- Tag resources with environment and project names
- Set up budget alerts on cloud providers
- Test infrastructure changes in dev first

## 🆘 Support

- **Pulumi**: [Slack Community](https://slack.pulumi.com/)
- **DuraGraph**: [GitHub Discussions](https://github.com/Duragraph/duragraph/discussions)
- **Issues**: [GitHub Issues](https://github.com/Duragraph/duragraph/issues)
