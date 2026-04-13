//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

func newCronJob(assistantID string, enabled bool) *postgres.CronJob {
	return &postgres.CronJob{
		AssistantID:    assistantID,
		Schedule:       "0 * * * *",
		Timezone:       "UTC",
		Payload:        map[string]interface{}{},
		Metadata:       map[string]interface{}{},
		Enabled:        enabled,
		OnRunCompleted: "keep",
	}
}

func TestCronRepository_CreateAndGetByID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCronRepository(testPool)
	assistantID := mustCreateAssistant(t, ctx)

	cron := &postgres.CronJob{
		AssistantID:    assistantID,
		Schedule:       "*/5 * * * *",
		Timezone:       "UTC",
		Payload:        map[string]interface{}{"action": "run"},
		Metadata:       map[string]interface{}{"env": "test"},
		Enabled:        true,
		OnRunCompleted: "schedule_next",
	}

	cronID, err := repo.Create(ctx, cron)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, err := repo.GetByID(ctx, cronID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if found.Schedule != "*/5 * * * *" {
		t.Errorf("schedule = %q", found.Schedule)
	}
	if !found.Enabled {
		t.Error("expected enabled")
	}
}

func TestCronRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCronRepository(testPool)
	assistantID := mustCreateAssistant(t, ctx)

	cron := newCronJob(assistantID, true)
	cronID, err := repo.Create(ctx, cron)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, cronID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	found, _ := repo.GetByID(ctx, cronID)
	if found != nil {
		t.Error("expected nil after delete")
	}
}

func TestCronRepository_Search(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCronRepository(testPool)
	assistantID := mustCreateAssistant(t, ctx)

	for i := 0; i < 3; i++ {
		cron := newCronJob(assistantID, i%2 == 0)
		if _, err := repo.Create(ctx, cron); err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
	}

	all, err := repo.Search(ctx, &assistantID, nil, nil, 10, 0, "created_at", "desc")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("len = %d, want 3", len(all))
	}

	enabled := true
	filtered, err := repo.Search(ctx, nil, nil, &enabled, 10, 0, "created_at", "desc")
	if err != nil {
		t.Fatalf("Search enabled: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("filtered len = %d, want 2", len(filtered))
	}
}

func TestCronRepository_UpdateNextRun(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCronRepository(testPool)
	assistantID := mustCreateAssistant(t, ctx)

	cron := newCronJob(assistantID, true)
	cronID, err := repo.Create(ctx, cron)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	nextRun := time.Now().Add(1 * time.Hour)
	if err := repo.UpdateNextRun(ctx, cronID, nextRun); err != nil {
		t.Fatalf("UpdateNextRun: %v", err)
	}

	found, _ := repo.GetByID(ctx, cronID)
	if found.NextRunDate == nil {
		t.Fatal("next_run_date is nil")
	}
}

func TestCronRepository_Count(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCronRepository(testPool)
	assistantID := mustCreateAssistant(t, ctx)

	for i := 0; i < 4; i++ {
		cron := newCronJob(assistantID, true)
		if _, err := repo.Create(ctx, cron); err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
	}

	count, err := repo.Count(ctx, &assistantID, nil)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 4 {
		t.Errorf("count = %d, want 4", count)
	}
}
