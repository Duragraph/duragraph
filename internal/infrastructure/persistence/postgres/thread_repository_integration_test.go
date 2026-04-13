//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

func TestThreadRepository_SaveAndFindByID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewThreadRepository(testPool, es)

	thread, err := workflow.NewThread(map[string]interface{}{"topic": "testing"})
	if err != nil {
		t.Fatalf("NewThread: %v", err)
	}

	if err := repo.Save(ctx, thread); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, thread.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.ID() != thread.ID() {
		t.Errorf("id = %q, want %q", found.ID(), thread.ID())
	}
	md := found.Metadata()
	if md["topic"] != "testing" {
		t.Errorf("metadata topic = %v, want testing", md["topic"])
	}
}

func TestThreadRepository_List(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewThreadRepository(testPool, es)

	for i := 0; i < 3; i++ {
		th, _ := workflow.NewThread(nil)
		repo.Save(ctx, th)
	}

	list, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("len = %d, want 3", len(list))
	}
}

func TestThreadRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewThreadRepository(testPool, es)

	th, _ := workflow.NewThread(nil)
	repo.Save(ctx, th)

	if err := repo.Delete(ctx, th.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, th.ID())
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestThreadRepository_Count(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewThreadRepository(testPool, es)

	for i := 0; i < 4; i++ {
		th, _ := workflow.NewThread(nil)
		repo.Save(ctx, th)
	}

	count, err := repo.Count(ctx, workflow.ThreadSearchFilters{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 4 {
		t.Errorf("count = %d, want 4", count)
	}
}

func TestThreadRepository_WithMessages(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	repo := postgres.NewThreadRepository(testPool, es)

	th, _ := workflow.NewThread(nil)
	th.AddMessage("user", "Hello!", nil)
	th.AddMessage("assistant", "Hi there!", nil)

	if err := repo.Save(ctx, th); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, th.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	msgs := found.Messages()
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello!" {
		t.Errorf("msg[0] = %+v", msgs[0])
	}

	_ = pkguuid.New()
}
