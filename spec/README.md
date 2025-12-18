# DuraGraph Technical Specifications

This folder contains all technical specifications for spec-driven development.

> **Creating specs for a new product?** See [SPEC_GENERATION_GUIDE.yml](SPEC_GENERATION_GUIDE.yml) for the complete workflow.

## Structure

```
spec/
├── api/                    # OpenAPI specifications
│   └── openapi.yml         # LangGraph-compatible REST API definitions
│
├── async/                  # AsyncAPI specifications
│   └── asyncapi.yml        # NATS JetStream event definitions
│
├── models/                 # Data models
│   ├── entities.yml        # Domain entities (Run, Assistant, Thread, Graph)
│   ├── events.yml          # Event schemas for event sourcing
│   └── dto.yml             # Request/Response DTOs
│
├── outbox/                 # Transactional outbox pattern
│   ├── schema.sql          # Outbox table definitions
│   └── outbox.yml          # Worker configuration and event mapping
│
├── observability/          # Observability specifications
│   ├── metrics.yml         # Prometheus metrics definitions
│   └── traces.yml          # Distributed tracing specifications
│
├── workers/                # Worker specifications
│   └── go-workers.yml      # Go worker daemon definitions
│
├── auth/                   # Authentication & Authorization
│   ├── auth.yml            # OAuth2, JWT configuration
│   └── rate-limiting.yml   # Rate limiting configuration
│
├── backend/                # Backend Development Guide
│   └── development-guide.yml  # Conventions, patterns, git
│
└── README.md               # This file
```

## Overview

DuraGraph is a LangGraph Cloud-compatible API for reliable AI workflow orchestration.

### Core Technologies
- **Language**: Go 1.23+
- **Framework**: Echo v4 (HTTP server)
- **Database**: PostgreSQL 15 with pgx/v5 driver
- **Message Broker**: NATS JetStream
- **Eventing**: Watermill (event bus)
- **Monitoring**: Prometheus metrics

### Architecture

```
┌─────────────────┐
│   Client SDKs   │  (LangGraph SDK, custom clients)
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  API Server (Go/Echo)               │
│  ├── LangGraph-compatible REST API  │
│  ├── SSE Streaming                  │
│  └── Prometheus Metrics             │
└────────┬────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  Event Store (PostgreSQL)           │
│  ├── Event Sourcing                 │
│  ├── Outbox Pattern                 │
│  └── CQRS Projections               │
└────────┬────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  NATS JetStream                     │
│  ├── Event Distribution             │
│  ├── Worker Coordination            │
│  └── Reliable Delivery              │
└────────┬────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  Worker Daemons                     │
│  ├── Graph Execution                │
│  ├── LLM Node Processing            │
│  └── Tool Execution                 │
└─────────────────────────────────────┘
```

## Conventions

### API Versioning
- REST APIs are versioned via URL path: `/api/v1/...`
- Breaking changes require new version

### Event Naming
- Format: `{domain}.{entity}.{action}`
- Examples: `run.created`, `run.completed`, `run.failed`

### Data Models
- All timestamps in UTC (ISO 8601)
- UUIDs for primary keys
- Soft deletes with `deleted_at` field

### Outbox Pattern
- Used for reliable event publishing
- Guarantees at-least-once delivery
- Events processed in order per aggregate

### Event Sourcing
- All state changes persisted as immutable events
- Aggregates reconstructed by replaying events
- Snapshots for performance optimization

### CQRS
- Commands: Write operations (side effects)
- Queries: Read-only operations (no side effects)
- Projections for optimized reads

### Metric Naming
- Format: `duragraph_{subsystem}_{name}_{unit}`
- Example: `duragraph_http_request_duration_seconds`

## Current Implementation Status

### Completed
- [x] LangGraph Cloud-compatible REST API (core endpoints)
- [x] Event sourcing with CQRS pattern
- [x] Graph execution engine
- [x] Outbox pattern for reliable event delivery
- [x] PostgreSQL event store
- [x] NATS JetStream messaging
- [x] Server-Sent Events streaming
- [x] OAuth2 integration (Google, GitHub)
- [x] Rate limiting (in-memory and Redis)
- [x] Redis caching layer

### In Progress
- [ ] Svelte dashboard visualization

### Planned
- [ ] Worker daemon processes
- [ ] Enhanced LLM provider support
- [ ] Multi-tenant support
- [ ] Production Helm charts
