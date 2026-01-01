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
   cd docker-compose/local-dev
   docker compose up -d
   ```

2. **Run your first example:**
   ```bash
   cd python/01-hello-world
   pip install -r requirements.txt
   python main.py
   ```

## Examples

### Python

| Example | Description | Difficulty |
|---------|-------------|------------|
| [01-hello-world](python/01-hello-world) | Minimal worker setup | Beginner |
| [02-chatbot](python/02-chatbot) | Conversational agent with memory | Beginner |
| [03-rag-agent](python/03-rag-agent) | RAG with vector store | Intermediate |
| [04-multi-agent](python/04-multi-agent) | Agent collaboration | Advanced |
| [05-human-in-loop](python/05-human-in-loop) | Approval workflows | Intermediate |
| [06-tool-use](python/06-tool-use) | Function calling | Intermediate |
| [07-evals](python/07-evals) | Running evaluations | Intermediate |

### Go

| Example | Description | Difficulty |
|---------|-------------|------------|
| [01-hello-world](go/01-hello-world) | Minimal Go worker | Beginner |
| [02-data-pipeline](go/02-data-pipeline) | High-performance pipeline | Intermediate |

### Docker Compose

| Example | Description |
|---------|-------------|
| [local-dev](docker-compose/local-dev) | Complete local development stack |
| [production](docker-compose/production) | Production-ready configuration |

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

## Related Repositories

| Repository | Description |
|------------|-------------|
| [duragraph](https://github.com/Duragraph/duragraph) | Core API server |
| [duragraph-python](https://github.com/Duragraph/duragraph-python) | Python SDK |
| [duragraph-go](https://github.com/Duragraph/duragraph-go) | Go SDK |
| [duragraph-docs](https://github.com/Duragraph/duragraph-docs) | Documentation |

## Documentation

- [Full Documentation](https://duragraph.dev/docs)
- [Python SDK](https://github.com/Duragraph/duragraph-python)
- [Go SDK](https://github.com/Duragraph/duragraph-go)

## Contributing

See [CONTRIBUTING.md](https://github.com/Duragraph/.github/blob/main/CONTRIBUTING.md) for guidelines.

### Adding a New Example

1. Create a new directory under the appropriate language folder
2. Include required files: `README.md`, `main.py`/`main.go`, dependencies file
3. Test against the latest DuraGraph version
4. Submit a PR

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
