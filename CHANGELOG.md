# Changelog

All notable changes to DuraGraph will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-04-13

### Added

- Full LangGraph Cloud API parity — assistants, threads, runs, streaming endpoints
- Event sourcing and CQRS architecture with PostgreSQL, NATS, and Redis
- Worker registration, heartbeat, and task assignment protocol
- MCP server with Streamable HTTP transport
- Crons API for scheduled run execution
- Store API for namespaced key-value storage
- Prometheus metrics and OpenTelemetry tracing
- Rate limiting middleware with configurable env vars
- Horizontal scaling safety for multi-instance deployment
- SSE streaming reliability with per-run NATS topics
- Comprehensive test suite (~54% coverage) across all layers
- Integration tests for PostgreSQL, NATS, and Redis
- GoReleaser pipeline with ko, Cosign signing, SBOM generation
- GitHub Actions CI/CD (tests, conformance, contracts, CodeQL)

### Fixed

- Canonical Apache 2.0 license
- Panic on short model names in LLM provider routing

[0.2.0]: https://github.com/Duragraph/duragraph/releases/tag/v0.2.0
