//go:build integration

package cache_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/infrastructure/cache"
)

func redisAddr() string {
	if u := os.Getenv("TEST_REDIS_ADDR"); u != "" {
		return u
	}
	return "127.0.0.1:6380"
}

func newTestCache(t *testing.T) *cache.RedisCache {
	t.Helper()
	c, err := cache.NewRedisCache(redisAddr(), "", 15)
	if err != nil {
		t.Fatalf("NewRedisCache: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	c.Client().FlushDB(context.Background())
	return c
}

func TestRedisCache_SetAndGet(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	err := c.Set(ctx, "test:key1", map[string]string{"name": "alice"}, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, err := c.Get(ctx, "test:key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", val)
	}
	if m["name"] != "alice" {
		t.Errorf("name = %v", m["name"])
	}
}

func TestRedisCache_GetString(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	c.Set(ctx, "test:str", "hello world", 5*time.Minute)

	val, err := c.GetString(ctx, "test:str")
	if err != nil {
		t.Fatalf("GetString: %v", err)
	}
	if val != `"hello world"` {
		t.Errorf("val = %q", val)
	}
}

func TestRedisCache_Delete(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	c.Set(ctx, "test:del", "value", 5*time.Minute)

	if err := c.Delete(ctx, "test:del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := c.Get(ctx, "test:del")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestRedisCache_Exists(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	exists, _ := c.Exists(ctx, "test:nope")
	if exists {
		t.Error("expected false for nonexistent key")
	}

	c.Set(ctx, "test:exists", "yes", 5*time.Minute)
	exists, _ = c.Exists(ctx, "test:exists")
	if !exists {
		t.Error("expected true for existing key")
	}
}

func TestRedisCache_Incr(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	v1, err := c.Incr(ctx, "test:counter")
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if v1 != 1 {
		t.Errorf("v1 = %d, want 1", v1)
	}

	v2, _ := c.Incr(ctx, "test:counter")
	if v2 != 2 {
		t.Errorf("v2 = %d, want 2", v2)
	}
}

func TestRedisCache_Expire(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	c.Set(ctx, "test:ttl", "val", 0)

	if err := c.Expire(ctx, "test:ttl", 1*time.Second); err != nil {
		t.Fatalf("Expire: %v", err)
	}

	exists, _ := c.Exists(ctx, "test:ttl")
	if !exists {
		t.Error("key should still exist")
	}

	time.Sleep(1500 * time.Millisecond)

	exists, _ = c.Exists(ctx, "test:ttl")
	if exists {
		t.Error("key should have expired")
	}
}

func TestRedisStateStore(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	store := cache.NewRedisStateStore(c)

	err := store.Set(ctx, "state-abc", map[string]string{"redirect": "http://example.com"}, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, err := store.Get(ctx, "state-abc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val == nil {
		t.Fatal("expected non-nil")
	}

	if err := store.Delete(ctx, "state-abc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	val, err = store.Get(ctx, "state-abc")
	if err == nil && val != nil {
		t.Error("expected nil or error after delete")
	}
}
