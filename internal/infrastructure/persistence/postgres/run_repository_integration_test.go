//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

func TestRunRepository_SaveAndFindByID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	assistantID := mustCreateAssistant(t, ctx)
	threadID := mustCreateThread(t, ctx)

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewRunRepository(testPool, es)

	r, err := run.NewRun(threadID, assistantID, map[string]interface{}{"msg": "hello"})
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}

	if err := repo.Save(ctx, r); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, r.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.ThreadID() != threadID {
		t.Errorf("threadID = %q, want %q", found.ThreadID(), threadID)
	}
	if found.AssistantID() != assistantID {
		t.Errorf("assistantID = %q, want %q", found.AssistantID(), assistantID)
	}
	if found.Status() != "queued" {
		t.Errorf("status = %q, want queued", found.Status())
	}
}

func TestRunRepository_FindByIDConsistent(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	assistantID := mustCreateAssistant(t, ctx)
	threadID := mustCreateThread(t, ctx)

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewRunRepository(testPool, es)

	r, _ := run.NewRun(threadID, assistantID, nil)
	repo.Save(ctx, r)

	found, err := repo.FindByIDConsistent(ctx, r.ID())
	if err != nil {
		t.Fatalf("FindByIDConsistent: %v", err)
	}
	if found.ID() != r.ID() {
		t.Errorf("id mismatch")
	}
}

func TestRunRepository_FindAll(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	assistantID := mustCreateAssistant(t, ctx)
	threadID := mustCreateThread(t, ctx)

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewRunRepository(testPool, es)

	for i := 0; i < 3; i++ {
		r, _ := run.NewRun(threadID, assistantID, nil)
		repo.Save(ctx, r)
	}

	runs, err := repo.FindAll(ctx, 10, 0)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("len = %d, want 3", len(runs))
	}
}

func TestRunRepository_FindByThreadID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	assistantID := mustCreateAssistant(t, ctx)
	threadID := mustCreateThread(t, ctx)

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewRunRepository(testPool, es)

	r, _ := run.NewRun(threadID, assistantID, nil)
	repo.Save(ctx, r)

	runs, err := repo.FindByThreadID(ctx, threadID, 10, 0)
	if err != nil {
		t.Fatalf("FindByThreadID: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("len = %d, want 1", len(runs))
	}
}

func TestRunRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	assistantID := mustCreateAssistant(t, ctx)
	threadID := mustCreateThread(t, ctx)

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewRunRepository(testPool, es)

	r, _ := run.NewRun(threadID, assistantID, nil)
	repo.Save(ctx, r)

	if err := repo.Delete(ctx, r.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, r.ID())
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestRunRepository_NotFound(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewRunRepository(testPool, es)

	_, err := repo.FindByID(ctx, "00000000-0000-4000-8000-000000000000")
	if err == nil {
		t.Error("expected not found error")
	}
}
