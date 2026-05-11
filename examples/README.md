# DuraGraph Examples

[![DuraGraph](https://img.shields.io/badge/DuraGraph-latest-blue)](https://github.com/Duragraph/duragraph)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Real-world examples demonstrating [DuraGraph](https://github.com/Duragraph/duragraph) capabilities for AI workflow orchestration.

## Prerequisites

- [DuraGraph](https://github.com/Duragraph/duragraph) control plane running
- Python 3.10+ (for Python examples)
- Go 1.21+ (for Go examples)
- Docker & Docker Compose (for infrastructure examples)

## Quick Start

1. **Start DuraGraph locally:**
   ```bash
   duragraph dev
   # → engine + dashboard on http://localhost:8081, embedded Postgres + NATS
   ```

2. **Run your first example:**

   > Python examples are run with `uv` — never `pip install`, never `python -m venv`. The editable-install path below points at the in-monorepo SDK source (`python/`); if you only want to *run* the example, `uv add duragraph` from PyPI works too.

   ```bash
   cd python/01-hello-world
   DURAGRAPH_URL=http://localhost:8081 PYTHONUNBUFFERED=1 \
     uv run --with-editable ../../../python \
     python main.py
   ```

   See [`python/01-hello-world/README.md`](python/01-hello-world/README.md) for the full demo cheat sheet (SSH port forwarding, end-to-end run, event-store inspection).

## Examples

### Implementation Progress

**Milestone v0.5 (SDK & Studio MVP):**
- ✅ Python Hello World
- ✅ Chatbot with Memory
- ✅ RAG Agent
- ✅ Go Hello World

**Milestone v0.8 (Production Ready):**
- 🚧 Multi-agent collaboration
- 🚧 Human-in-the-loop workflows
- 🚧 Tool use patterns
- 🚧 Evaluation framework

### Python

| Example | Description | Difficulty | Status |
|---------|-------------|------------|--------|
| [01-hello-world](python/01-hello-world) | Minimal worker setup | Beginner | ✅ Complete |
| [02-chatbot](python/02-chatbot) | Conversational agent with memory | Beginner | ✅ Complete |
| [03-rag-agent](python/03-rag-agent) | RAG with vector store | Intermediate | ✅ Complete |
| [04-multi-agent](python/04-multi-agent) | Agent collaboration | Advanced | 🚧 Planned |
| [05-human-in-loop](python/05-human-in-loop) | Approval workflows | Intermediate | 🚧 Planned |
| [06-tool-use](python/06-tool-use) | Function calling | Intermediate | 🚧 Planned |
| [07-evals](python/07-evals) | Running evaluations | Intermediate | 🚧 Planned |

### Go

| Example | Description | Difficulty | Status |
|---------|-------------|------------|--------|
| [01-hello-world](go/01-hello-world) | Minimal Go worker | Beginner | ✅ Complete |
| [02-data-pipeline](go/02-data-pipeline) | High-performance pipeline | Intermediate | 🚧 Planned |

### Docker Compose

| Example | Description | Status |
|---------|-------------|--------|
| [local-dev](docker-compose/local-dev) | Complete local development stack | ✅ Complete |
| [production](docker-compose/production) | Production-ready configuration | 🚧 Planned |

## Structure

```
duragraph-examples/
├── python/
│   ├── 01-hello-world/
│   ├── 02-chatbot/
│   ├── 03-rag-agent/
│   ├── 04-multi-agent/
│   ├── 05-human-in-loop/
│   ├── 06-tool-use/
│   └── 07-evals/
├── go/
│   ├── 01-hello-world/
│   └── 02-data-pipeline/
├── docker-compose/
│   ├── local-dev/
│   └── production/
└── README.md
```

## Related modules

Examples live in the [`duragraph`](https://github.com/Duragraph/duragraph) monorepo alongside the engine and SDKs:

| Path | Description |
|------|-------------|
| [`/`](https://github.com/Duragraph/duragraph) | Core engine (Go) |
| [`python/`](https://github.com/Duragraph/duragraph/tree/main/python) | Python SDK — PyPI: [`duragraph`](https://pypi.org/project/duragraph/) |
| [`go-sdk/`](https://github.com/Duragraph/duragraph/tree/main/go-sdk) | Go SDK — [`pkg.go.dev`](https://pkg.go.dev/github.com/duragraph/duragraph/go-sdk) |
| [`docs/`](https://github.com/Duragraph/duragraph/tree/main/docs) | Documentation site source |

## Documentation

- [Full Documentation](https://duragraph.ai/docs)
- [Python SDK (`python/`)](https://github.com/Duragraph/duragraph/tree/main/python)
- [Go SDK (`go-sdk/`)](https://github.com/Duragraph/duragraph/tree/main/go-sdk)

## Contributing

See [CONTRIBUTING.md](https://github.com/Duragraph/duragraph/blob/main/CONTRIBUTING.md) for guidelines.

### Adding a New Example

1. Create a new directory under the appropriate language folder
2. Include required files: `README.md`, `main.py`/`main.go`, dependencies file
3. Test against the latest DuraGraph version
4. Submit a PR

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
