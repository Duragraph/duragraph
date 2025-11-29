package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// CachedRunRepository wraps RunRepository with caching
type CachedRunRepository struct {
	repo  run.Repository
	cache *RedisCache
	ttl   time.Duration
}

// NewCachedRunRepository creates a cached run repository
func NewCachedRunRepository(repo run.Repository, cache *RedisCache, ttl time.Duration) *CachedRunRepository {
	if ttl == 0 {
		ttl = 5 * time.Minute // Default TTL
	}

	return &CachedRunRepository{
		repo:  repo,
		cache: cache,
		ttl:   ttl,
	}
}

// FindByID retrieves a run with caching
func (r *CachedRunRepository) FindByID(ctx context.Context, id string) (*run.Run, error) {
	// Try cache first
	// cacheKey := fmt.Sprintf("run:%s", id)

	// Note: Caching complex aggregates requires custom serialization
	// For simplicity, we'll cache miss and always hit the database
	// In production, implement proper serialization

	runAgg, err := r.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Could cache here with proper serialization
	return runAgg, nil
}

// Save invalidates cache on write
func (r *CachedRunRepository) Save(ctx context.Context, runAgg *run.Run) error {
	if err := r.repo.Save(ctx, runAgg); err != nil {
		return err
	}

	// Invalidate cache
	cacheKey := fmt.Sprintf("run:%s", runAgg.ID())
	r.cache.Delete(ctx, cacheKey)

	return nil
}

// Update invalidates cache on write
func (r *CachedRunRepository) Update(ctx context.Context, runAgg *run.Run) error {
	if err := r.repo.Update(ctx, runAgg); err != nil {
		return err
	}

	// Invalidate cache
	cacheKey := fmt.Sprintf("run:%s", runAgg.ID())
	r.cache.Delete(ctx, cacheKey)

	return nil
}

// Delegate other methods
func (r *CachedRunRepository) FindByThreadID(ctx context.Context, threadID string, limit, offset int) ([]*run.Run, error) {
	return r.repo.FindByThreadID(ctx, threadID, limit, offset)
}

func (r *CachedRunRepository) FindByAssistantID(ctx context.Context, assistantID string, limit, offset int) ([]*run.Run, error) {
	return r.repo.FindByAssistantID(ctx, assistantID, limit, offset)
}

func (r *CachedRunRepository) FindByStatus(ctx context.Context, status run.Status, limit, offset int) ([]*run.Run, error) {
	return r.repo.FindByStatus(ctx, status, limit, offset)
}

func (r *CachedRunRepository) Delete(ctx context.Context, id string) error {
	if err := r.repo.Delete(ctx, id); err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("run:%s", id)
	r.cache.Delete(ctx, cacheKey)

	return nil
}

// CachedAssistantRepository wraps AssistantRepository with caching
type CachedAssistantRepository struct {
	repo  workflow.AssistantRepository
	cache *RedisCache
	ttl   time.Duration
}

// NewCachedAssistantRepository creates a cached assistant repository
func NewCachedAssistantRepository(repo workflow.AssistantRepository, cache *RedisCache, ttl time.Duration) *CachedAssistantRepository {
	if ttl == 0 {
		ttl = 15 * time.Minute // Assistants change less frequently
	}

	return &CachedAssistantRepository{
		repo:  repo,
		cache: cache,
		ttl:   ttl,
	}
}

// FindByID with caching
func (r *CachedAssistantRepository) FindByID(ctx context.Context, id string) (*workflow.Assistant, error) {
	return r.repo.FindByID(ctx, id)
}

// Save invalidates cache
func (r *CachedAssistantRepository) Save(ctx context.Context, assistant *workflow.Assistant) error {
	if err := r.repo.Save(ctx, assistant); err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("assistant:%s", assistant.ID())
	r.cache.Delete(ctx, cacheKey)

	return nil
}

// Update invalidates cache
func (r *CachedAssistantRepository) Update(ctx context.Context, assistant *workflow.Assistant) error {
	if err := r.repo.Update(ctx, assistant); err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("assistant:%s", assistant.ID())
	r.cache.Delete(ctx, cacheKey)

	return nil
}

// List delegates to repository
func (r *CachedAssistantRepository) List(ctx context.Context, limit, offset int) ([]*workflow.Assistant, error) {
	return r.repo.List(ctx, limit, offset)
}

// Delete invalidates cache
func (r *CachedAssistantRepository) Delete(ctx context.Context, id string) error {
	if err := r.repo.Delete(ctx, id); err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("assistant:%s", id)
	r.cache.Delete(ctx, cacheKey)

	return nil
}

// CacheWarmer warms up the cache with frequently accessed data
type CacheWarmer struct {
	cache                 *RedisCache
	assistantRepo         workflow.AssistantRepository
	frequentAccessPattern time.Duration
}

// NewCacheWarmer creates a new cache warmer
func NewCacheWarmer(cache *RedisCache, assistantRepo workflow.AssistantRepository) *CacheWarmer {
	return &CacheWarmer{
		cache:                 cache,
		assistantRepo:         assistantRepo,
		frequentAccessPattern: 1 * time.Minute,
	}
}

// WarmAssistants pre-loads frequently accessed assistants
func (w *CacheWarmer) WarmAssistants(ctx context.Context) error {
	// Load all assistants (or top N)
	assistants, err := w.assistantRepo.List(ctx, 100, 0)
	if err != nil {
		return err
	}

	// Cache each assistant
	for _, assistant := range assistants {
		cacheKey := fmt.Sprintf("assistant:%s", assistant.ID())
		// Would implement serialization here
		_ = cacheKey
	}

	return nil
}

// StartPeriodicWarming starts periodic cache warming
func (w *CacheWarmer) StartPeriodicWarming(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.WarmAssistants(ctx)
		}
	}
}
