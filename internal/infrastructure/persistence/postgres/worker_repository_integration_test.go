//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

func TestWorkerRepository_SaveAndFindByID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewWorkerRepository(testPool)

	w := &worker.Worker{
		ID:            pkguuid.New(),
		Name:          "test-worker",
		Status:        worker.StatusReady,
		Capabilities:  worker.Capabilities{},
		ActiveRuns:    0,
		TotalRuns:     0,
		FailedRuns:    0,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}

	if err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.Name != "test-worker" {
		t.Errorf("name = %q", found.Name)
	}
	if found.Status != worker.StatusReady {
		t.Errorf("status = %q", found.Status)
	}
}

func TestWorkerRepository_FindAll(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewWorkerRepository(testPool)

	for i := 0; i < 3; i++ {
		w := &worker.Worker{
			ID:            pkguuid.New(),
			Name:          "worker",
			Status:        worker.StatusReady,
			LastHeartbeat: time.Now(),
			RegisteredAt:  time.Now(),
		}
		repo.Save(ctx, w)
	}

	workers, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(workers) != 3 {
		t.Errorf("len = %d, want 3", len(workers))
	}
}

func TestWorkerRepository_Heartbeat(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewWorkerRepository(testPool)

	w := &worker.Worker{
		ID:            pkguuid.New(),
		Name:          "hb-worker",
		Status:        worker.StatusReady,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}
	repo.Save(ctx, w)

	if err := repo.Heartbeat(ctx, w.ID, worker.StatusRunning, 2, 10, 1); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	found, _ := repo.FindByID(ctx, w.ID)
	if found.Status != worker.StatusRunning {
		t.Errorf("status = %q, want running", found.Status)
	}
	if found.ActiveRuns != 2 {
		t.Errorf("active_runs = %d, want 2", found.ActiveRuns)
	}
}

func TestWorkerRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewWorkerRepository(testPool)

	w := &worker.Worker{
		ID:            pkguuid.New(),
		Name:          "del-worker",
		Status:        worker.StatusReady,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}
	repo.Save(ctx, w)

	if err := repo.Delete(ctx, w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, w.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestWorkerRepository_FindHealthy(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewWorkerRepository(testPool)

	healthy := &worker.Worker{
		ID:            pkguuid.New(),
		Name:          "healthy",
		Status:        worker.StatusReady,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}
	repo.Save(ctx, healthy)

	stale := &worker.Worker{
		ID:            pkguuid.New(),
		Name:          "stale",
		Status:        worker.StatusReady,
		LastHeartbeat: time.Now().Add(-2 * time.Hour),
		RegisteredAt:  time.Now(),
	}
	repo.Save(ctx, stale)

	workers, err := repo.FindHealthy(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("FindHealthy: %v", err)
	}
	if len(workers) != 1 {
		t.Errorf("len = %d, want 1", len(workers))
	}
}

func TestWorkerRepository_CleanupStale(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewWorkerRepository(testPool)

	stale := &worker.Worker{
		ID:            pkguuid.New(),
		Name:          "stale",
		Status:        worker.StatusReady,
		LastHeartbeat: time.Now().Add(-48 * time.Hour),
		RegisteredAt:  time.Now(),
	}
	repo.Save(ctx, stale)

	fresh := &worker.Worker{
		ID:            pkguuid.New(),
		Name:          "fresh",
		Status:        worker.StatusReady,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}
	repo.Save(ctx, fresh)

	deleted, err := repo.CleanupStale(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupStale: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
}

func TestWorkerRepository_Upsert(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewWorkerRepository(testPool)

	w := &worker.Worker{
		ID:            pkguuid.New(),
		Name:          "original",
		Status:        worker.StatusReady,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}
	repo.Save(ctx, w)

	w.Name = "updated"
	w.Status = worker.StatusRunning
	repo.Save(ctx, w)

	found, _ := repo.FindByID(ctx, w.ID)
	if found.Name != "updated" {
		t.Errorf("name = %q, want updated", found.Name)
	}
}
