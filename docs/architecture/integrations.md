# DuraGraph Integrations

This document clarifies which integrations are available in which component.

## Integration Matrix

| Category | Integration | Control Plane | SDK (Python) | Status |
|----------|-------------|:-------------:|:------------:|--------|
| **Database** | PostgreSQL | ✅ | ❌ | ✅ Available |
| **Messaging** | NATS | ✅ | ❌ | ✅ Available |
| **LLM** | OpenAI | ❌ | ✅ | ✅ Available |
| **LLM** | Anthropic | ❌ | ✅ | ✅ Available |
| **LLM** | Cohere | ❌ | ✅ | 🔜 Planned |
| **LLM** | Ollama | ❌ | ✅ | 🔜 Planned |
| **Vector DB** | Chroma | ❌ | ✅ | 🔜 Planned |
| **Vector DB** | Pinecone | ❌ | ✅ | 🔜 Planned |
| **Vector DB** | Weaviate | ❌ | ✅ | 🔜 Planned |
| **Vector DB** | Qdrant | ❌ | ✅ | 🔜 Planned |
| **Vector DB** | pgvector | ❌ | ✅ | 🔜 Planned |
| **Vector DB** | Milvus | ❌ | ✅ | 🔜 Planned |
| **Graph DB** | Neo4j | ❌ | ✅ | 🔜 Planned |
| **Graph DB** | Amazon Neptune | ❌ | ✅ | 🔜 Planned |
| **Graph DB** | Apache AGE | ❌ | ✅ | 🔜 Planned |
| **Embeddings** | OpenAI | ❌ | ✅ | 🔜 Planned |
| **Embeddings** | Cohere | ❌ | ✅ | 🔜 Planned |
| **Embeddings** | Local (sentence-transformers) | ❌ | ✅ | 🔜 Planned |
| **Observability** | Prometheus | ✅ | ❌ | ✅ Available |
| **Observability** | OpenTelemetry | 🔜 | 🔜 | 🔜 Planned |

## Control Plane Integrations

The control plane intentionally has minimal external dependencies:

```yaml
# Control Plane Dependencies
required:
  - PostgreSQL 15+    # State storage, event store
  - NATS JetStream    # Internal messaging

optional:
  - Prometheus        # Metrics export
  - OpenTelemetry     # Distributed tracing (planned)
```

### Why Minimal Dependencies?

1. **Operational Simplicity** - Fewer moving parts to manage
2. **Security** - No external API keys stored in control plane
3. **Reliability** - Core orchestration doesn't depend on external services
4. **Flexibility** - Users choose their own providers in workers

## SDK Integrations

The SDK is where all external AI/ML integrations live:

### LLM Providers

```python
from duragraph.llm import get_provider

# OpenAI
provider = get_provider("gpt-4o-mini")

# Anthropic
provider = get_provider("claude-3-sonnet")

# Automatic detection from model name
```

**Supported Models:**
- OpenAI: gpt-4o, gpt-4o-mini, gpt-4-turbo, gpt-3.5-turbo, o1-*, o3-*
- Anthropic: claude-3-opus, claude-3-sonnet, claude-3-haiku, claude-3.5-*

### Vector Stores

```python
from duragraph.vectorstores import Chroma, Pinecone, Weaviate

# Local development
store = Chroma(collection="docs")

# Production
store = Pinecone(index="prod-docs", api_key="...")
store = Weaviate(url="...", api_key="...")
```

### Knowledge Graphs

```python
from duragraph.graphs import Neo4j, Neptune

# Neo4j
kg = Neo4j(uri="bolt://localhost:7687", auth=("neo4j", "password"))

# Amazon Neptune
kg = Neptune(endpoint="...", region="us-east-1")
```

### Embeddings

```python
from duragraph.embeddings import OpenAIEmbeddings, LocalEmbeddings

# Cloud
embedder = OpenAIEmbeddings(model="text-embedding-3-small")

# Local (no API calls)
embedder = LocalEmbeddings(model="all-MiniLM-L6-v2")
```

## Installation

### Control Plane

```bash
# Docker Compose (recommended)
docker compose up -d

# Includes PostgreSQL and NATS automatically
```

### SDK with Integrations

```bash
# Base SDK
pip install duragraph-python

# With specific integrations
pip install duragraph-python[openai]      # OpenAI LLM + embeddings
pip install duragraph-python[anthropic]   # Anthropic LLM
pip install duragraph-python[pinecone]    # Pinecone vector store
pip install duragraph-python[chroma]      # Chroma vector store
pip install duragraph-python[neo4j]       # Neo4j graph database

# All integrations
pip install duragraph-python[all]
```

## Configuration

### Control Plane

```bash
# Environment variables
DB_HOST=localhost
DB_PORT=5432
DB_USER=appuser
DB_PASSWORD=apppass
DB_NAME=appdb
NATS_URL=nats://localhost:4222
```

### SDK / Workers

```bash
# LLM API keys (in worker environment)
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...

# Vector store keys (in worker environment)
PINECONE_API_KEY=...
WEAVIATE_API_KEY=...

# Graph database (in worker environment)
NEO4J_URI=bolt://localhost:7687
NEO4J_PASSWORD=...
```

## FAQ

### Why don't you support [X] in the control plane?

The control plane is designed to be a simple orchestrator. Adding LLM/vector/graph integrations would:
- Require storing API keys centrally (security risk)
- Add operational complexity
- Limit flexibility (users locked into our choices)

### Can I use a provider you don't support?

Yes! The SDK provides abstract base classes:
- `LLMProvider` - Implement for custom LLMs
- `VectorStore` - Implement for custom vector DBs
- `KnowledgeGraph` - Implement for custom graph DBs

### Will you add [X] integration?

Open an issue! We prioritize based on community demand.

### Can I run without the control plane?

Yes! The SDK supports local execution for development:

```python
agent = MyAgent()
result = agent.run({"message": "Hello"})  # No control plane needed
```
