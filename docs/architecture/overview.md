# DuraGraph Architecture Overview

DuraGraph is a platform for building reliable AI workflows. It consists of two main components:

1. **Control Plane** - Orchestration server (Go)
2. **SDK/Workers** - Client libraries for building agents (Python, etc.)

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CONTROL PLANE                                   │
│                            (duragraph - Go)                                  │
│                                                                              │
│   ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────────┐ │
│   │   HTTP API      │  │  Run Scheduler  │  │      Event Store            │ │
│   │   /api/v1/*     │  │  & Orchestrator │  │  (PostgreSQL)               │ │
│   └─────────────────┘  └─────────────────┘  └─────────────────────────────┘ │
│                                                                              │
│   Responsibilities:                                                          │
│   ✓ Run orchestration          ✓ State management                           │
│   ✓ Assistants/Threads/Runs    ✓ Event streaming (SSE)                      │
│   ✓ Worker registration        ✓ Checkpointing                              │
│                                                                              │
│   Does NOT handle:                                                           │
│   ✗ LLM calls                  ✗ Vector search                              │
│   ✗ Tool execution             ✗ Knowledge graph queries                    │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      │ HTTP/SSE
                                      │
┌─────────────────────────────────────┴───────────────────────────────────────┐
│                                WORKERS                                       │
│                         (duragraph-python SDK)                               │
│                                                                              │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                        Your Agent Code                               │   │
│   │   @Graph(id="my_agent")                                             │   │
│   │   class MyAgent:                                                     │   │
│   │       @llm_node(model="gpt-4o-mini")                                │   │
│   │       def process(self, state): ...                                  │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                      │                                       │
│   ┌──────────────────────────────────┴──────────────────────────────────┐   │
│   │                         SDK Integrations                             │   │
│   │                                                                      │   │
│   │   ┌─────────────┐   ┌─────────────┐   ┌─────────────────────────┐  │   │
│   │   │ LLM         │   │ Vector      │   │ Knowledge               │  │   │
│   │   │ Providers   │   │ Stores      │   │ Graphs                  │  │   │
│   │   │             │   │             │   │                         │  │   │
│   │   │ • OpenAI    │   │ • Chroma    │   │ • Neo4j                 │  │   │
│   │   │ • Anthropic │   │ • Pinecone  │   │ • Neptune               │  │   │
│   │   │ • Cohere    │   │ • Weaviate  │   │ • Apache AGE            │  │   │
│   │   │ • Ollama    │   │ • Qdrant    │   │                         │  │   │
│   │   │             │   │ • pgvector  │   │                         │  │   │
│   │   └──────┬──────┘   └──────┬──────┘   └────────────┬────────────┘  │   │
│   │          │                 │                       │               │   │
│   └──────────┼─────────────────┼───────────────────────┼───────────────┘   │
│              │                 │                       │                    │
└──────────────┼─────────────────┼───────────────────────┼────────────────────┘
               │                 │                       │
               ▼                 ▼                       ▼
        ┌─────────────┐   ┌─────────────┐   ┌─────────────────────────┐
        │ OpenAI API  │   │ Pinecone    │   │ Neo4j                   │
        │ Anthropic   │   │ Weaviate    │   │ Graph Database          │
        │ etc.        │   │ etc.        │   │                         │
        └─────────────┘   └─────────────┘   └─────────────────────────┘
           External           External              External
           Services           Services              Services
```

## Component Responsibilities

### Control Plane (duragraph)

| Responsibility | Description |
|----------------|-------------|
| **Run Orchestration** | Create, schedule, and manage workflow runs |
| **State Management** | Store and retrieve thread/run state |
| **Worker Management** | Register workers, assign runs, health checks |
| **Event Streaming** | Server-Sent Events for real-time updates |
| **Checkpointing** | Save/restore execution state |
| **API Gateway** | LangGraph-compatible REST API |

**Storage:** PostgreSQL only (runs, threads, assistants, events)

### Workers / SDK (duragraph-python)

| Responsibility | Description |
|----------------|-------------|
| **Graph Definition** | Define workflows with decorators |
| **LLM Integration** | Call OpenAI, Anthropic, etc. |
| **Vector Search** | Query Pinecone, Chroma, etc. |
| **Knowledge Graphs** | Query Neo4j, Neptune, etc. |
| **Tool Execution** | Run tools/functions |
| **Local Execution** | Run graphs without control plane |

**Connections:** LLM APIs, Vector DBs, Graph DBs (user-provided)

## What Connects Where?

| Integration | Location | Why |
|-------------|----------|-----|
| **PostgreSQL** | Control Plane | Core state storage |
| **NATS** | Control Plane | Internal messaging |
| **OpenAI/Anthropic** | SDK/Worker | LLM calls happen in workers |
| **Pinecone/Weaviate** | SDK/Worker | Vector search in workers |
| **Neo4j/Neptune** | SDK/Worker | Graph queries in workers |
| **Redis** (future) | Either | Caching (optional) |

## Data Flow

```
1. User creates a Run via Control Plane API
   └─▶ Control Plane stores run in PostgreSQL

2. Worker polls for work
   └─▶ Control Plane assigns run to worker

3. Worker executes graph
   ├─▶ Calls LLM (OpenAI/Anthropic)
   ├─▶ Queries Vector DB (Pinecone/etc)
   ├─▶ Queries Knowledge Graph (Neo4j/etc)
   └─▶ Sends events back to Control Plane

4. Control Plane streams events to client
   └─▶ Client receives real-time updates
```

## Why This Architecture?

### Control Plane is "Dumb"
- Only handles orchestration
- No vendor lock-in for LLMs/vectors
- Simple to operate (just PostgreSQL)
- Scales horizontally

### Workers are "Smart"
- All integrations live in SDK
- Users bring their own API keys
- Flexible - use any provider
- Can run locally for development

### Benefits
1. **Separation of Concerns** - Orchestration vs execution
2. **Flexibility** - Choose your own LLM/vector/graph providers
3. **Simplicity** - Control plane has minimal dependencies
4. **Security** - API keys stay with workers, not control plane
