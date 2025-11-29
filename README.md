# DuraGraph

![DuraGraph Logo](docs/public/duragraph_logo.png)

**An open, extensible orchestration layer for AI and workflow automation**

DuraGraph provides a **LangGraph Cloud-compatible API** built with **Event Sourcing** and **CQRS** patterns for reliable, observable, and maintainable AI pipelines that can be self-hosted in enterprise environments.

## 🎯 Mission

Enable reliable, observable, and maintainable AI pipelines that feel natural for developers—bringing the power of LangGraph Cloud to self-hosted and enterprise environments with:

- **API Compatibility**: Drop-in replacement for LangGraph Cloud APIs
- **Enterprise Ready**: Self-hosted, compliant, secure
- **Fault Tolerant**: Event sourcing with reliable event delivery via outbox pattern
- **Observable**: Rich monitoring and workflow introspection

## 🚀 Quick Start

Get started with DuraGraph in minutes:

**📖 [View Documentation](https://duragraph.dev/docs)** | **🎓 [Quick Start Guide](https://duragraph.dev/docs/getting-started)**

### One-Click Deploy

Deploy DuraGraph to your preferred cloud platform:

[![Deploy on Fly.io](https://fly.io/static/images/fly-logo.svg)](https://fly.io/docs)
[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy)
[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/template)
[![Deploy on DigitalOcean](https://www.deploytodo.com/do-btn-blue.svg)](https://cloud.digitalocean.com/apps/new)
[![Deploy on Scaleway](https://www.scaleway.com/en/docs/_astro/logo-scaleway.svg)](https://www.scaleway.com/en/docs/)

### Local Development

```bash
# Clone the repository
git clone https://github.com/Duragraph/duragraph.git
cd duragraph

# Start all services with Docker Compose
docker-compose up -d

# Or use Task
brew install go-task/tap/go-task
task up
```

Visit **http://localhost:8080** for the API and **http://localhost:5173** for the dashboard.

**👉 [Full Setup Guide](https://duragraph.dev/docs/getting-started)**

## 🏗️ Architecture

```mermaid
flowchart LR
  client["Client SDKs / LangGraph Cloud clients"]
  api[API Server - Go/Echo]
  eventstore[(Event Store - PostgreSQL)]
  nats[NATS JetStream]
  engine[Graph Execution Engine]
  dashboard[Svelte Dashboard]

  client --> api
  api --> eventstore
  api --> engine
  eventstore --> nats
  nats --> dashboard
  engine --> eventstore
```

**🔧 [Architecture Details](https://duragraph.dev/docs/architecture)**

## ⚡ Key Features

- 🔄 **LangGraph Cloud API Compatible** - Drop-in replacement for existing LangGraph Cloud integrations
- 🏢 **Self-Hosted** - Full control over your data and infrastructure
- ⚡ **Event Sourcing & CQRS** - Reliable, auditable workflow execution with event-driven architecture
- 🔍 **Observable** - Rich monitoring, tracing, and debugging tools with Prometheus metrics
- 🧩 **Extensible** - Custom graph execution engine with support for LLM nodes and tool execution
- 📊 **Visual Dashboard** - Real-time workflow visualization with Server-Sent Events
- 🐳 **Docker Ready** - Easy deployment with Docker Compose or Kubernetes

## 📚 Documentation

- **[Getting Started](https://duragraph.dev/docs/getting-started)** - Installation and basic usage
- **[API Reference](https://duragraph.dev/docs/api)** - Complete API documentation
- **[Architecture](https://duragraph.dev/docs/architecture)** - System design and components
- **[Development Guide](https://duragraph.dev/docs/development)** - Contributing and development
- **[Deployment](https://duragraph.dev/docs/deployment)** - Production deployment guides
- **[Operations](https://duragraph.dev/docs/ops)** - Monitoring and maintenance

## 🔧 Basic Usage

```python
from langgraph_sdk import get_client

# Point to your DuraGraph instance
client = get_client(url="http://localhost:8080")

# Use exactly like LangGraph Cloud
assistant = await client.assistants.create(...)
thread = await client.threads.create()
run = await client.runs.create(
    thread_id=thread["id"],
    assistant_id=assistant["id"]
)
```

**📖 [Full API Documentation](https://duragraph.dev/docs/api)**

## 🗂️ Project Structure

```
duragraph/
├── cmd/server/          # API server (Go)
├── internal/
│   ├── domain/          # Domain models (aggregates, entities, events)
│   ├── application/     # Use cases (commands, queries, services)
│   ├── infrastructure/  # External concerns (HTTP, persistence, messaging)
│   └── pkg/             # Shared utilities (errors, eventbus, uuid)
├── dashboard/           # Svelte visualization dashboard
├── website/             # Landing page (Vite/React)
├── docs/                # Documentation (Fumadocs/Next.js)
├── deploy/              # Docker, SQL migrations
└── Taskfile.yml         # Development task runner
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes and add tests
4. Run tests: `task test`
5. Open a Pull Request

**🛠️ [Development Guide](https://duragraph.dev/docs/development)**

## 📄 License

Licensed under the [Apache License 2.0](LICENSE).

## 🙋 Support

- **Documentation**: [duragraph.dev/docs](https://duragraph.dev/docs)
- **Issues**: [GitHub Issues](https://github.com/Duragraph/duragraph/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Duragraph/duragraph/discussions)

## 🗺️ Roadmap

- [x] LangGraph Cloud-compatible API
- [x] Event sourcing with CQRS pattern
- [x] Custom graph execution engine
- [x] Outbox pattern for reliable event delivery
- [x] PostgreSQL event store with NATS JetStream messaging
- [x] Fumadocs documentation site
- [x] Svelte dashboard for visualization
- [x] Server-Sent Events streaming
- [ ] Enhanced LLM provider support (additional models)
- [ ] Advanced workflow patterns (parallel execution, subgraphs)
- [ ] Production Helm charts
- [ ] Multi-tenant support
- [ ] Workflow versioning and migration tools

**📋 [Full Roadmap](https://duragraph.dev/docs/roadmap)**

---

**DuraGraph** - Bringing enterprise-grade AI workflow orchestration to everyone.

**[Get Started](https://duragraph.dev/docs/getting-started)** · **[Documentation](https://duragraph.dev/docs)** · **[Community](https://github.com/Duragraph/duragraph/discussions)**
