# DuraGraph Go SDK

Enterprise-grade Go SDK for AI workflow orchestration.

## Installation

```bash
go get github.com/duragraph/duragraph-go
```

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/duragraph/duragraph-go/graph"
    "github.com/duragraph/duragraph-go/worker"
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

## Documentation

See [docs.duragraph.io](https://docs.duragraph.io) for full documentation.

## License

Apache-2.0
