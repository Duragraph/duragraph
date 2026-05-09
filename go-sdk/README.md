# DuraGraph Go SDK

[![CI](https://github.com/Duragraph/duragraph/actions/workflows/go-sdk-ci.yml/badge.svg)](https://github.com/Duragraph/duragraph/actions/workflows/go-sdk-ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/duragraph/duragraph/go-sdk.svg)](https://pkg.go.dev/github.com/duragraph/duragraph/go-sdk)
[![Go Report Card](https://img.shields.io/badge/go%20report-A%2B-brightgreen)](https://goreportcard.com/report/github.com/duragraph/duragraph/go-sdk)
[![License](https://img.shields.io/github/license/Duragraph/duragraph)](https://github.com/Duragraph/duragraph/blob/main/LICENSE)
[![GitHub Stars](https://img.shields.io/github/stars/Duragraph/duragraph?style=social)](https://github.com/Duragraph/duragraph/stargazers)

Go SDK for [DuraGraph](https://github.com/Duragraph/duragraph) - Reliable AI Workflow Orchestration.

Build AI agents with structs and interfaces, deploy to a control plane, and get full observability out of the box.

## Installation

```bash
go get github.com/duragraph/duragraph/go-sdk
```

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/duragraph/duragraph/go-sdk/graph"
    "github.com/duragraph/duragraph/go-sdk/worker"
)

// Define your state
type ChatState struct {
    Messages []string `json:"messages"`
    Result   string   `json:"result,omitempty"`
}

// Define a node
type ThinkNode struct{}

func (n *ThinkNode) Execute(ctx context.Context, state *ChatState) (*ChatState, error) {
    state.Result = "Hello from Go!"
    return state, nil
}

func main() {
    // Create graph
    g := graph.New[ChatState]("my_agent")
    g.AddNode("think", &ThinkNode{})
    g.SetEntrypoint("think")

    // Run locally
    result, err := g.Run(context.Background(), &ChatState{
        Messages: []string{"Hello"},
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Println(result.Result)

    // Or connect to control plane
    w := worker.New(g,
        worker.WithControlPlane("http://localhost:8081"),
    )
    w.Start(context.Background())
}
```

## Features

- **Graph Definition** - Define workflows with structs and interfaces
- **REST API Client** - Full control plane client for assistants, threads, runs, store, and crons
- **LLM Providers** - OpenAI, Anthropic, Gemini, Ollama, Cohere
- **Vector Stores** - Chroma, Pinecone, Weaviate, Qdrant, Milvus, Elasticsearch, pgvector
- **Knowledge Graphs** - Neo4j, Memgraph, ArangoDB
- **Document Storage** - S3, GCS, Azure Blob
- **Observability** - OpenTelemetry, Prometheus metrics
- **Worker Runtime** - Connect to DuraGraph control plane

## Requirements

- Go 1.21+
- DuraGraph Control Plane (for deployment)

## Documentation

- [REST API Client Reference](docs/client-api.md)
- [Full Documentation](https://duragraph.ai/docs)
- [API Reference](https://duragraph.ai/docs/api-reference/overview)
- [Examples](https://github.com/Duragraph/duragraph/tree/main/examples)

## Related Components

The Go SDK lives in `go-sdk/` inside the [Duragraph monorepo](https://github.com/Duragraph/duragraph). Other components in the same repo:

| Path | Description |
|------|-------------|
| [`/`](https://github.com/Duragraph/duragraph) | Core API server (Go) |
| [`python/`](https://github.com/Duragraph/duragraph/tree/main/python) | Python SDK |
| [`examples/`](https://github.com/Duragraph/duragraph/tree/main/examples) | Example projects |
| [`docs/`](https://github.com/Duragraph/duragraph/tree/main/docs) | Documentation site |

## Contributing

See [CONTRIBUTING.md](https://github.com/Duragraph/.github/blob/main/CONTRIBUTING.md) for guidelines.

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
