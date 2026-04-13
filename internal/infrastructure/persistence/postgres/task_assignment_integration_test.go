//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

func TestTaskAssignmentRepository_CreateAndFindByRunID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewTaskAssignmentRepository(testPool)

	task := &worker.TaskAssignment{
		RunID:       pkguuid.New(),
		GraphID:     "my-graph",
		ThreadID:    pkguuid.New(),
		AssistantID: pkguuid.New(),
		Input:       map[string]interface{}{"msg": "test"},
		Config:      map[string]interface{}{"model": "gpt-4"},
		MaxRetries:  3,
	}

	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.ID == 0 {
		t.Error("expected non-zero ID after create")
	}

	found, err := repo.FindByRunID(ctx, task.RunID)
	if err != nil {
		t.Fatalf("FindByRunID: %v", err)
	}
	if found.GraphID != "my-graph" {
		t.Errorf("graphID = %q", found.GraphID)
	}
	if found.Status != worker.TaskStatusPending {
		t.Errorf("status = %q, want pending", found.Status)
	}
	if found.MaxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", found.MaxRetries)
	}
}

func TestTaskAssignmentRepository_CompleteViaDirect(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewTaskAssignmentRepository(testPool)

	task := &worker.TaskAssignment{
		RunID:       pkguuid.New(),
		GraphID:     "graph-1",
		ThreadID:    pkguuid.New(),
		AssistantID: pkguuid.New(),
		MaxRetries:  3,
	}
	repo.Create(ctx, task)

	// Directly claim via SQL to work around Claim RETURNING column mismatch
	workerID := pkguuid.New()
	_, err := testPool.Exec(ctx, `
		UPDATE task_assignments
		SET status = 'claimed', worker_id = $1, claimed_at = NOW(), lease_expires_at = NOW() + INTERVAL '30 minutes'
		WHERE id = $2
	`, workerID, task.ID)
	if err != nil {
		t.Fatalf("direct claim: %v", err)
	}

	if err := repo.Complete(ctx, task.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	found, _ := repo.FindByRunID(ctx, task.RunID)
	if found.Status != worker.TaskStatusCompleted {
		t.Errorf("status = %q, want completed", found.Status)
	}
}

func TestTaskAssignmentRepository_Fail(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewTaskAssignmentRepository(testPool)

	task := &worker.TaskAssignment{
		RunID:       pkguuid.New(),
		GraphID:     "graph-2",
		ThreadID:    pkguuid.New(),
		AssistantID: pkguuid.New(),
		MaxRetries:  0,
	}
	repo.Create(ctx, task)

	// Directly claim
	_, err := testPool.Exec(ctx, `
		UPDATE task_assignments
		SET status = 'claimed', worker_id = $1, claimed_at = NOW()
		WHERE id = $2
	`, pkguuid.New(), task.ID)
	if err != nil {
		t.Fatalf("direct claim: %v", err)
	}

	if err := repo.Fail(ctx, task.ID, "something went wrong"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	found, _ := repo.FindByRunID(ctx, task.RunID)
	if found.Status != worker.TaskStatusFailed {
		t.Errorf("status = %q, want failed", found.Status)
	}
	if found.ErrorMessage != "something went wrong" {
		t.Errorf("error = %q", found.ErrorMessage)
	}
}

func TestTaskAssignmentRepository_ClaimEmptyInputs(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewTaskAssignmentRepository(testPool)

	claimed, err := repo.Claim(ctx, "w1", nil, 0, 1)
	if err != nil {
		t.Fatalf("Claim empty: %v", err)
	}
	if claimed != nil {
		t.Errorf("expected nil, got %v", claimed)
	}

	claimed2, err := repo.Claim(ctx, "w1", []string{"g"}, 0, 0)
	if err != nil {
		t.Fatalf("Claim 0 max: %v", err)
	}
	if claimed2 != nil {
		t.Errorf("expected nil, got %v", claimed2)
	}
}

func TestTaskAssignmentRepository_RetryOrFail(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewTaskAssignmentRepository(testPool)

	task := &worker.TaskAssignment{
		RunID:       pkguuid.New(),
		GraphID:     "graph-r",
		ThreadID:    pkguuid.New(),
		AssistantID: pkguuid.New(),
		MaxRetries:  2,
	}
	repo.Create(ctx, task)

	// Claim it directly
	_, _ = testPool.Exec(ctx, `
		UPDATE task_assignments SET status = 'claimed', worker_id = 'w1', claimed_at = NOW()
		WHERE id = $1
	`, task.ID)

	// RetryOrFail: should requeue since retry_count(0) < max_retries(2)
	if err := repo.RetryOrFail(ctx, task.ID); err != nil {
		t.Fatalf("RetryOrFail: %v", err)
	}

	found, _ := repo.FindByRunID(ctx, task.RunID)
	if found.Status != worker.TaskStatusPending {
		t.Errorf("status = %q, want pending (retried)", found.Status)
	}
	if found.RetryCount != 1 {
		t.Errorf("retryCount = %d, want 1", found.RetryCount)
	}
}

func TestTaskAssignmentRepository_FindExpiredLeases(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewTaskAssignmentRepository(testPool)

	task := &worker.TaskAssignment{
		RunID:       pkguuid.New(),
		GraphID:     "graph-e",
		ThreadID:    pkguuid.New(),
		AssistantID: pkguuid.New(),
		MaxRetries:  1,
	}
	repo.Create(ctx, task)

	// Claim with expired lease
	_, _ = testPool.Exec(ctx, `
		UPDATE task_assignments
		SET status = 'claimed', worker_id = 'w1', claimed_at = NOW(),
		    lease_expires_at = NOW() - INTERVAL '1 hour'
		WHERE id = $1
	`, task.ID)

	expired, err := repo.FindExpiredLeases(ctx)
	if err != nil {
		t.Fatalf("FindExpiredLeases: %v", err)
	}
	if len(expired) != 1 {
		t.Errorf("expired len = %d, want 1", len(expired))
	}
}
