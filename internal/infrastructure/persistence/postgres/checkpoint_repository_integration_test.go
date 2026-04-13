//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/checkpoint"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

func TestCheckpointRepository_SaveAndFindByID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCheckpointRepository(testPool)
	threadID := mustCreateThread(t, ctx)

	cp := checkpoint.Reconstitute(
		pkguuid.New(),
		threadID,
		"default",
		pkguuid.New(),
		"",
		map[string]interface{}{"messages": []interface{}{"hello"}},
		map[string]int{"messages": 1},
		map[string]map[string]int{"node1": {"messages": 1}},
		nil,
		timeNow(),
	)

	if err := repo.Save(ctx, cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, cp.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.ThreadID() != threadID {
		t.Errorf("threadID = %q", found.ThreadID())
	}
}

func TestCheckpointRepository_FindLatest(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCheckpointRepository(testPool)
	threadID := mustCreateThread(t, ctx)

	cp1 := checkpoint.Reconstitute(pkguuid.New(), threadID, "ns", "cp1", "", nil, nil, nil, nil, timeNow())
	cp2 := checkpoint.Reconstitute(pkguuid.New(), threadID, "ns", "cp2", "cp1", nil, nil, nil, nil, timeNow())

	repo.Save(ctx, cp1)
	repo.Save(ctx, cp2)

	latest, err := repo.FindLatest(ctx, threadID, "ns")
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	if latest.CheckpointID() != "cp2" {
		t.Errorf("latest = %q, want cp2", latest.CheckpointID())
	}
}

func TestCheckpointRepository_FindByCheckpointID(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCheckpointRepository(testPool)
	threadID := mustCreateThread(t, ctx)

	cpID := "my-checkpoint-id"
	cp := checkpoint.Reconstitute(pkguuid.New(), threadID, "ns", cpID, "", nil, nil, nil, nil, timeNow())
	repo.Save(ctx, cp)

	found, err := repo.FindByCheckpointID(ctx, threadID, "ns", cpID)
	if err != nil {
		t.Fatalf("FindByCheckpointID: %v", err)
	}
	if found.CheckpointID() != cpID {
		t.Errorf("cpID = %q", found.CheckpointID())
	}
}

func TestCheckpointRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCheckpointRepository(testPool)
	threadID := mustCreateThread(t, ctx)

	cp := checkpoint.Reconstitute(pkguuid.New(), threadID, "ns", "cpd", "", nil, nil, nil, nil, timeNow())
	repo.Save(ctx, cp)

	if err := repo.Delete(ctx, cp.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, cp.ID())
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestCheckpointRepository_FindHistory(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCheckpointRepository(testPool)
	threadID := mustCreateThread(t, ctx)

	for i := 0; i < 5; i++ {
		cp := checkpoint.Reconstitute(pkguuid.New(), threadID, "ns", pkguuid.New(), "", nil, nil, nil, nil, timeNow())
		repo.Save(ctx, cp)
	}

	history, err := repo.FindHistory(ctx, threadID, "ns", 3, "")
	if err != nil {
		t.Fatalf("FindHistory: %v", err)
	}
	if len(history) != 3 {
		t.Errorf("len = %d, want 3", len(history))
	}
}

func TestCheckpointRepository_SaveWrite(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewCheckpointRepository(testPool)
	threadID := mustCreateThread(t, ctx)

	cpID := "cp-for-write"
	cp := checkpoint.Reconstitute(pkguuid.New(), threadID, "ns", cpID, "", nil, nil, nil, nil, timeNow())
	repo.Save(ctx, cp)

	write := checkpoint.NewCheckpointWrite(threadID, "ns", cpID, "task1", 0, "messages", "put", map[string]interface{}{"msg": "hi"})

	if err := repo.SaveWrite(ctx, write); err != nil {
		t.Fatalf("SaveWrite: %v", err)
	}

	writes, err := repo.FindWritesByCheckpoint(ctx, threadID, "ns", cpID)
	if err != nil {
		t.Fatalf("FindWritesByCheckpoint: %v", err)
	}
	if len(writes) != 1 {
		t.Errorf("writes len = %d, want 1", len(writes))
	}
}
