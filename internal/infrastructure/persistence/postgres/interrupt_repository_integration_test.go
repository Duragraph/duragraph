//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/humanloop"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

func mustCreateRun(t *testing.T, ctx context.Context) string {
	t.Helper()
	assistantID := mustCreateAssistant(t, ctx)
	threadID := mustCreateThread(t, ctx)
	runID := pkguuid.New()
	_, err := testPool.Exec(ctx, `
		INSERT INTO runs (id, thread_id, assistant_id, status, input, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, 'queued', '{}', '{}', NOW(), NOW())
	`, runID, threadID, assistantID)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return runID
}

func TestInterruptRepository_SaveAndFindByID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewInterruptRepository(testPool, es)

	runID := mustCreateRun(t, ctx)
	state := map[string]interface{}{"step": "review"}
	toolCalls := []map[string]interface{}{{"name": "search", "args": map[string]interface{}{"q": "test"}}}

	interrupt, err := humanloop.NewInterrupt(runID, "node-1", humanloop.ReasonApprovalRequired, state, toolCalls)
	if err != nil {
		t.Fatalf("NewInterrupt: %v", err)
	}

	if err := repo.Save(ctx, interrupt); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, interrupt.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.RunID() != runID {
		t.Errorf("runID = %q", found.RunID())
	}
}

func TestInterruptRepository_FindByRunID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewInterruptRepository(testPool, es)

	runID := mustCreateRun(t, ctx)

	i1, _ := humanloop.NewInterrupt(runID, "n1", humanloop.ReasonToolCall, nil, nil)
	i2, _ := humanloop.NewInterrupt(runID, "n2", humanloop.ReasonInputNeeded, nil, nil)
	repo.Save(ctx, i1)
	repo.Save(ctx, i2)

	interrupts, err := repo.FindByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("FindByRunID: %v", err)
	}
	if len(interrupts) != 2 {
		t.Errorf("len = %d, want 2", len(interrupts))
	}
}

func TestInterruptRepository_FindUnresolvedByRunID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewInterruptRepository(testPool, es)

	runID := mustCreateRun(t, ctx)

	i1, _ := humanloop.NewInterrupt(runID, "n1", humanloop.ReasonToolCall, nil, nil)
	repo.Save(ctx, i1)

	unresolved, err := repo.FindUnresolvedByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("FindUnresolvedByRunID: %v", err)
	}
	if len(unresolved) != 1 {
		t.Errorf("len = %d, want 1", len(unresolved))
	}
}

func TestInterruptRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewInterruptRepository(testPool, es)

	runID := mustCreateRun(t, ctx)
	interrupt, _ := humanloop.NewInterrupt(runID, "n1", humanloop.ReasonToolCall, nil, nil)
	repo.Save(ctx, interrupt)

	if err := repo.Delete(ctx, interrupt.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, interrupt.ID())
	if err == nil {
		t.Error("expected error after delete")
	}
}
