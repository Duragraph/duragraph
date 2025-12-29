// Package duragraph provides the Go SDK for DuraGraph AI workflow orchestration.
//
// DuraGraph is an enterprise-grade platform for building reliable AI agents
// with support for multiple LLM providers, vector stores, and knowledge graphs.
//
// # Quick Start
//
//	import "github.com/duragraph/duragraph-go/graph"
//
//	g := graph.New[MyState]("my_agent")
//	g.AddNode("think", &ThinkNode{})
//	g.SetEntrypoint("think")
//	result, _ := g.Run(ctx, &MyState{})
//
// See https://docs.duragraph.io for full documentation.
package duragraph

// Version is the current SDK version.
const Version = "0.1.0"
