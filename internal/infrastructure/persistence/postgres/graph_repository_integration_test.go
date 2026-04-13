//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

func TestGraphRepository_SaveAndFindByID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewGraphRepository(testPool, es)

	assistantID := mustCreateAssistant(t, ctx)

	nodes := []workflow.Node{
		{ID: "__start__", Type: "start"},
		{ID: "process", Type: "llm"},
		{ID: "__end__", Type: "end"},
	}
	edges := []workflow.Edge{
		{Source: "__start__", Target: "process"},
		{Source: "process", Target: "__end__"},
	}

	graph, err := workflow.NewGraph(assistantID, "test-graph", "1.0.0", "A test graph", nodes, edges, nil)
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	if err := repo.Save(ctx, graph); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, graph.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.Name() != "test-graph" {
		t.Errorf("name = %q, want %q", found.Name(), "test-graph")
	}
}

func TestGraphRepository_FindByAssistantID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewGraphRepository(testPool, es)

	assistantID := mustCreateAssistant(t, ctx)

	nodes := []workflow.Node{
		{ID: "__start__", Type: "start"},
		{ID: "__end__", Type: "end"},
	}
	edges := []workflow.Edge{
		{Source: "__start__", Target: "__end__"},
	}

	g1, _ := workflow.NewGraph(assistantID, "g1", "1.0.0", "", nodes, edges, nil)
	g2, _ := workflow.NewGraph(assistantID, "g2", "2.0.0", "", nodes, edges, nil)
	repo.Save(ctx, g1)
	repo.Save(ctx, g2)

	graphs, err := repo.FindByAssistantID(ctx, assistantID)
	if err != nil {
		t.Fatalf("FindByAssistantID: %v", err)
	}
	if len(graphs) != 2 {
		t.Errorf("len = %d, want 2", len(graphs))
	}
}

func TestGraphRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewGraphRepository(testPool, es)

	assistantID := mustCreateAssistant(t, ctx)

	nodes := []workflow.Node{
		{ID: "__start__", Type: "start"},
		{ID: "__end__", Type: "end"},
	}
	edges := []workflow.Edge{
		{Source: "__start__", Target: "__end__"},
	}

	g, _ := workflow.NewGraph(assistantID, "to-del", "1.0.0", "", nodes, edges, nil)
	repo.Save(ctx, g)

	if err := repo.Delete(ctx, g.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, g.ID())
	if err == nil {
		t.Error("expected error after delete")
	}
}
