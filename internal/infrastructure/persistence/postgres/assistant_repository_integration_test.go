//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

func TestAssistantRepository_SaveAndFindByID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewAssistantRepository(testPool, es)

	assistant, err := workflow.NewAssistant("test-bot", "A test bot", "gpt-4", "Be helpful", nil, map[string]interface{}{"env": "test"})
	if err != nil {
		t.Fatalf("NewAssistant: %v", err)
	}

	if err := repo.Save(ctx, assistant); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, assistant.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.Name() != "test-bot" {
		t.Errorf("name = %q, want %q", found.Name(), "test-bot")
	}
	if found.Model() != "gpt-4" {
		t.Errorf("model = %q, want %q", found.Model(), "gpt-4")
	}
}

func TestAssistantRepository_List(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewAssistantRepository(testPool, es)

	for i := 0; i < 3; i++ {
		a, _ := workflow.NewAssistant("bot-"+pkguuid.New()[:8], "desc", "gpt-4", "inst", nil, nil)
		if err := repo.Save(ctx, a); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	list, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("len = %d, want 3", len(list))
	}
}

func TestAssistantRepository_Update(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewAssistantRepository(testPool, es)

	a, _ := workflow.NewAssistant("old-name", "desc", "gpt-4", "inst", nil, nil)
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}

	updated, _ := workflow.ReconstructAssistant(
		a.ID(), "new-name", "new desc", "gpt-4o", "new inst",
		nil, nil, a.CreatedAt(), time.Now(),
	)

	if err := repo.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	found, _ := repo.FindByID(ctx, a.ID())
	if found.Name() != "new-name" {
		t.Errorf("name = %q, want %q", found.Name(), "new-name")
	}
}

func TestAssistantRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewAssistantRepository(testPool, es)

	a, _ := workflow.NewAssistant("to-delete", "desc", "gpt-4", "inst", nil, nil)
	repo.Save(ctx, a)

	if err := repo.Delete(ctx, a.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, a.ID())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestAssistantRepository_Count(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewAssistantRepository(testPool, es)

	for i := 0; i < 5; i++ {
		a, _ := workflow.NewAssistant("bot-"+pkguuid.New()[:8], "desc", "gpt-4", "inst", nil, nil)
		repo.Save(ctx, a)
	}

	count, err := repo.Count(ctx, workflow.AssistantSearchFilters{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestAssistantRepository_SaveVersion(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewAssistantRepository(testPool, es)

	a, _ := workflow.NewAssistant("versioned", "desc", "gpt-4", "inst", nil, nil)
	repo.Save(ctx, a)

	version := workflow.AssistantVersionInfo{
		ID:          pkguuid.New(),
		AssistantID: a.ID(),
		Version:     1,
		GraphID:     pkguuid.New(),
		Config:      map[string]interface{}{"key": "val"},
		Context:     []interface{}{"ctx1"},
		CreatedAt:   time.Now(),
	}

	if err := repo.SaveVersion(ctx, version); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}

	versions, err := repo.FindVersions(ctx, a.ID(), 10)
	if err != nil {
		t.Fatalf("FindVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("versions len = %d, want 1", len(versions))
	}
}
