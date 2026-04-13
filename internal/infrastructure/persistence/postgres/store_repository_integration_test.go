//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

func TestStoreRepository_PutAndGet(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewStoreRepository(testPool)

	ns := []string{"app", "settings"}
	val := map[string]interface{}{"theme": "dark", "lang": "en"}

	if err := repo.Put(ctx, ns, "prefs", val, 0); err != nil {
		t.Fatalf("Put: %v", err)
	}

	item, err := repo.Get(ctx, ns, "prefs", false)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item == nil {
		t.Fatal("Get returned nil")
	}
	if item.Key != "prefs" {
		t.Errorf("key = %q", item.Key)
	}
	if item.Value["theme"] != "dark" {
		t.Errorf("theme = %v", item.Value["theme"])
	}
}

func TestStoreRepository_PutWithTTL(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewStoreRepository(testPool)

	ns := []string{"cache"}
	if err := repo.Put(ctx, ns, "temp", map[string]interface{}{"v": 1}, 60); err != nil {
		t.Fatalf("Put: %v", err)
	}

	item, err := repo.Get(ctx, ns, "temp", false)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item == nil {
		t.Fatal("expected item with future TTL")
	}
}

func TestStoreRepository_Delete(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewStoreRepository(testPool)

	ns := []string{"test"}
	repo.Put(ctx, ns, "k1", map[string]interface{}{"a": 1}, 0)

	if err := repo.Delete(ctx, ns, "k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	item, _ := repo.Get(ctx, ns, "k1", false)
	if item != nil {
		t.Error("expected nil after delete")
	}
}

func TestStoreRepository_Search(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewStoreRepository(testPool)

	ns := []string{"org", "team"}
	repo.Put(ctx, ns, "user1", map[string]interface{}{"role": "admin"}, 0)
	repo.Put(ctx, ns, "user2", map[string]interface{}{"role": "member"}, 0)
	repo.Put(ctx, ns, "user3", map[string]interface{}{"role": "admin"}, 0)

	items, err := repo.Search(ctx, []string{"org"}, nil, 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("len = %d, want 3", len(items))
	}

	filtered, err := repo.Search(ctx, []string{"org"}, map[string]interface{}{"role": "admin"}, 10, 0)
	if err != nil {
		t.Fatalf("Search filtered: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("filtered len = %d, want 2", len(filtered))
	}
}

func TestStoreRepository_Upsert(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	repo := postgres.NewStoreRepository(testPool)

	ns := []string{"test"}
	repo.Put(ctx, ns, "k1", map[string]interface{}{"v": 1}, 0)
	repo.Put(ctx, ns, "k1", map[string]interface{}{"v": 2}, 0)

	item, _ := repo.Get(ctx, ns, "k1", false)
	if item.Value["v"] != float64(2) {
		t.Errorf("v = %v, want 2", item.Value["v"])
	}
}
