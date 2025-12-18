# DuraGraph vs LangGraph Cloud API - Gap Analysis

## Summary

| Category | LangGraph Cloud | DuraGraph | Gap |
|----------|----------------|-----------|-----|
| Assistants | 9 endpoints | 5 endpoints | 4 missing |
| Threads | 8 endpoints | 5 endpoints | 3 missing |
| Thread Runs | 10 endpoints | 3 endpoints | 7 missing |
| Stateless Runs | 4 endpoints | 0 endpoints | 4 missing |
| Crons | 4 endpoints | 0 endpoints | 4 missing (Phase 2) |
| Store | 4 endpoints | 0 endpoints | 4 missing (Phase 2) |
| MCP | 3 endpoints | 0 endpoints | 3 missing (Phase 2) |
| **TOTAL** | **42 endpoints** | **13 endpoints** | **29 missing** |

---

## Phase 1 Scope (Drop-in Replacement - No UI)

**Excluded from Phase 1:**
- Crons API (scheduled runs)
- Store API (key-value persistence)
- MCP API (model context protocol)
- Graph visualization endpoints
- UI/Dashboard

---

## Detailed Gap Analysis

### 1. ASSISTANTS API

| Endpoint | Method | LangGraph | DuraGraph | Status |
|----------|--------|-----------|-----------|--------|
| `/assistants` | POST | ✅ | ✅ | Implemented |
| `/assistants/search` | POST | ✅ | ❌ | **MISSING** |
| `/assistants/{assistant_id}` | GET | ✅ | ✅ | Implemented |
| `/assistants/{assistant_id}` | PATCH | ✅ | ✅ | Implemented |
| `/assistants/{assistant_id}` | DELETE | ✅ | ✅ | Implemented |
| `/assistants/{assistant_id}/graph` | GET | ✅ | ❌ | **MISSING** (Phase 2 - visualization) |
| `/assistants/{assistant_id}/schemas` | GET | ✅ | ❌ | **MISSING** |
| `/assistants/{assistant_id}/versions` | POST | ✅ | ❌ | **MISSING** |
| `/assistants/{assistant_id}/latest` | POST | ✅ | ❌ | **MISSING** |
| `/assistants/{assistant_id}/subgraphs` | GET | ✅ | ❌ | **MISSING** (Phase 2 - visualization) |
| `/assistants/{assistant_id}/subgraphs/{namespace}` | GET | ✅ | ❌ | **MISSING** (Phase 2 - visualization) |

**Phase 1 Missing:** search, schemas, versions, latest (4 endpoints)

### 2. THREADS API

| Endpoint | Method | LangGraph | DuraGraph | Status |
|----------|--------|-----------|-----------|--------|
| `/threads` | POST | ✅ | ✅ | Implemented |
| `/threads/search` | POST | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}` | GET | ✅ | ✅ | Implemented |
| `/threads/{thread_id}` | PATCH | ✅ | ✅ | Implemented |
| `/threads/{thread_id}` | DELETE | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/copy` | POST | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/state` | GET | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/state` | POST | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/state/{checkpoint_id}` | GET | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/state/checkpoint` | POST | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/history` | GET | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/history` | POST | ✅ | ❌ | **MISSING** |

**Phase 1 Missing:** search, delete, copy, state (GET/POST), state checkpoint, history (8 endpoints)

### 3. THREAD RUNS API

| Endpoint | Method | LangGraph | DuraGraph | Status |
|----------|--------|-----------|-----------|--------|
| `/threads/{thread_id}/runs` | POST | ✅ | ✅ (as `/runs`) | **NEEDS PATH CHANGE** |
| `/threads/{thread_id}/runs` | GET | ✅ | ✅ | Implemented |
| `/threads/{thread_id}/runs/stream` | POST | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/runs/wait` | POST | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/runs/{run_id}` | GET | ✅ | ✅ (as `/runs/:run_id`) | **NEEDS PATH CHANGE** |
| `/threads/{thread_id}/runs/{run_id}` | DELETE | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/runs/{run_id}/cancel` | POST | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/runs/{run_id}/join` | GET | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/runs/{run_id}/stream` | GET | ✅ | ❌ | **MISSING** |
| `/threads/{thread_id}/runs/crons` | POST | ✅ | ❌ | Phase 2 (Crons) |

**Phase 1 Missing:** stream, wait, delete, cancel, join, stream (existing) + path restructure (8 items)

### 4. STATELESS RUNS API

| Endpoint | Method | LangGraph | DuraGraph | Status |
|----------|--------|-----------|-----------|--------|
| `/runs` | POST | ✅ | ⚠️ | Exists but different semantics |
| `/runs/stream` | POST | ✅ | ❌ | **MISSING** |
| `/runs/wait` | POST | ✅ | ❌ | **MISSING** |
| `/runs/batch` | POST | ✅ | ❌ | **MISSING** |
| `/runs/cancel` | POST | ✅ | ❌ | **MISSING** |

**Phase 1 Missing:** stream, wait, batch, cancel (4 endpoints)

### 5. STREAMING

| Feature | LangGraph | DuraGraph | Status |
|---------|-----------|-----------|--------|
| `values` mode | ✅ | ❌ | **MISSING** |
| `messages` mode | ✅ | ❌ | **MISSING** |
| `updates` mode | ✅ | ❌ | **MISSING** |
| `events` mode | ✅ | ⚠️ | Basic SSE exists |
| `debug` mode | ✅ | ❌ | **MISSING** |
| Streaming via POST | ✅ | ❌ | **MISSING** (current is GET) |

### 6. STATE MANAGEMENT

| Feature | LangGraph | DuraGraph | Status |
|---------|-----------|-----------|--------|
| Checkpoints | ✅ | ❌ | **MISSING** |
| State history | ✅ | ❌ | **MISSING** |
| State updates | ✅ | ❌ | **MISSING** |
| Fork thread from checkpoint | ✅ | ❌ | **MISSING** |

### 7. SCHEMAS/MODELS

| Schema | LangGraph | DuraGraph | Status |
|--------|-----------|-----------|--------|
| Assistant | ✅ | ⚠️ | Partial - missing graph_id, metadata, version |
| Thread | ✅ | ⚠️ | Partial - missing metadata, values, status |
| Run | ✅ | ⚠️ | Partial - missing multitask_strategy, interrupt_before/after |
| ThreadState | ✅ | ❌ | **MISSING** |
| Checkpoint | ✅ | ❌ | **MISSING** |
| Config | ✅ | ❌ | **MISSING** |

---

## Phase 1 Issue Categories

### INFRA - Infrastructure (6 issues)
- INFRA-001: Project structure & GitHub setup
- INFRA-002: CI/CD pipeline (test, lint, build)
- INFRA-003: OpenAPI spec generation from LangGraph reference
- INFRA-004: Conformance test framework
- INFRA-005: Docker Compose dev environment validation
- INFRA-006: Database migrations for new schemas

### API - API Compatibility (12 issues)
- API-001: Restructure routes to match LangGraph paths
- API-002: Implement `/assistants/search` endpoint
- API-003: Implement `/assistants/{id}/schemas` endpoint
- API-004: Implement `/assistants/{id}/versions` endpoint
- API-005: Implement `/assistants/{id}/latest` endpoint
- API-006: Implement `/threads/search` endpoint
- API-007: Implement `/threads/{id}` DELETE endpoint
- API-008: Implement `/threads/{id}/copy` endpoint
- API-009: Implement stateless runs endpoints (`/runs/*`)
- API-010: Implement `/runs/batch` endpoint
- API-011: Implement `/runs/cancel` endpoint
- API-012: Update request/response DTOs to match LangGraph schemas

### STATE - State Management (6 issues)
- STATE-001: Implement checkpoint storage
- STATE-002: Implement `/threads/{id}/state` GET endpoint
- STATE-003: Implement `/threads/{id}/state` POST endpoint (update)
- STATE-004: Implement `/threads/{id}/state/{checkpoint_id}` endpoint
- STATE-005: Implement `/threads/{id}/state/checkpoint` POST endpoint
- STATE-006: Implement `/threads/{id}/history` endpoints

### STREAM - Streaming (6 issues)
- STREAM-001: Implement `values` streaming mode
- STREAM-002: Implement `messages` streaming mode (LLM tokens)
- STREAM-003: Implement `updates` streaming mode
- STREAM-004: Implement `debug` streaming mode
- STREAM-005: Convert streaming to POST endpoints
- STREAM-006: Implement `/threads/{id}/runs/{id}/stream` GET endpoint

### RUN - Run Lifecycle (6 issues)
- RUN-001: Implement run cancel functionality
- RUN-002: Implement `/threads/{id}/runs/{id}/join` endpoint
- RUN-003: Implement `/threads/{id}/runs/wait` endpoint
- RUN-004: Implement `/threads/{id}/runs/stream` POST endpoint
- RUN-005: Implement run deletion
- RUN-006: Implement interrupt_before/interrupt_after support

### GRAPH - Graph Execution (4 issues)
- GRAPH-001: Support graph config (configurable fields)
- GRAPH-002: Implement subgraph support
- GRAPH-003: Improve human-in-the-loop handling
- GRAPH-004: Support multitask_strategy (reject, interrupt, rollback, enqueue)

### TEST - Conformance Testing (4 issues)
- TEST-001: Expand conformance tests for all Assistant endpoints
- TEST-002: Expand conformance tests for all Thread endpoints
- TEST-003: Expand conformance tests for all Run endpoints
- TEST-004: Expand conformance tests for streaming modes

### DOCS - Documentation (3 issues)
- DOCS-001: Update OpenAPI spec to full LangGraph compatibility
- DOCS-002: API migration guide (from LangGraph Cloud)
- DOCS-003: Deployment documentation

---

## Phase 1 Milestone Summary

**Total Issues:** 47
- INFRA: 6
- API: 12
- STATE: 6
- STREAM: 6
- RUN: 6
- GRAPH: 4
- TEST: 4
- DOCS: 3

**Goal:** Full LangGraph Cloud API compatibility (minus Crons, Store, MCP, and visualization)

---

## Phase 2 (Future)

- Crons API (scheduled runs)
- Store API (key-value persistence)
- MCP API
- Graph visualization (`/graph`, `/subgraphs`)
- Dashboard UI
- Multi-tenant support
