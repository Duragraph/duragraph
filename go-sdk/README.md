# DuraGraph Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/Duragraph/duragraph-go.svg)](https://pkg.go.dev/github.com/Duragraph/duragraph-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/Duragraph/duragraph-go)](https://goreportcard.com/report/github.com/Duragraph/duragraph-go)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/Duragraph/duragraph-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Duragraph/duragraph-go/actions/workflows/ci.yml)

Go SDK for [DuraGraph](https://github.com/Duragraph/duragraph) - Reliable AI Workflow Orchestration.

Build AI agents with structs and interfaces, deploy to a control plane, and get full observability out of the box.

## Installation

```bash
go get github.com/Duragraph/duragraph-go
```

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/Duragraph/duragraph-go/graph"
    "github.com/Duragraph/duragraph-go/worker"
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

- [Full Documentation](https://duragraph.ai/docs)
- [API Reference](https://duragraph.ai/docs/api-reference/overview)
- [Examples](https://github.com/Duragraph/duragraph-examples)

## Related Repositories

| Repository | Description |
|------------|-------------|
| [duragraph](https://github.com/Duragraph/duragraph) | Core API server |
| [duragraph-python](https://github.com/Duragraph/duragraph-python) | Python SDK |
| [duragraph-examples](https://github.com/Duragraph/duragraph-examples) | Example projects |
| [duragraph-docs](https://github.com/Duragraph/duragraph-docs) | Documentation |

## Contributing

See [CONTRIBUTING.md](https://github.com/Duragraph/.github/blob/main/CONTRIBUTING.md) for guidelines.

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
