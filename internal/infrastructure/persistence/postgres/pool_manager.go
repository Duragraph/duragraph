package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbNameRegex matches the canonical per-tenant database name format:
// the literal "tenant_" prefix followed by a UUID's 32 hex characters
// with hyphens stripped (lowercase only). See models/entities.yml
// `tenants.db_name` for the schema-layer enforcement of this format
// and project memory `Tenancy implementation → DB naming convention`
// for the rationale (PG identifier safety + interpolation safety).
//
// TODO(wave-1): once internal/domain/tenant/dbname.go lands (separate
// PR), consolidate the helpers in this file (`tenantDBName`,
// `validateTenantDBName`, `dbNameRegex`) onto that domain package and
// have PoolManager call into it. Keeping the duplicate here until then
// so this PR can ship independently.
var dbNameRegex = regexp.MustCompile(`^tenant_[a-f0-9]{32}$`)

const (
	// defaultMaxConnsPerTenant caps connections inside a single tenant's
	// pgxpool. Two is enough for typical request fan-out (one for the
	// repository read, one for outbox/event-store write inside a tx)
	// while keeping aggregate connections to `prod-postgres` bounded —
	// 100 active tenants × 2 conns ≈ 200, comfortably under the
	// `max_connections=500` headroom set on the production instance.
	defaultMaxConnsPerTenant int32 = 2

	// defaultIdleTimeout closes a tenant's pool after it has been idle
	// for this long. Idle = `lastUsed` not bumped via ForTenant.
	defaultIdleTimeout = 10 * time.Minute

	// defaultEvictInterval is how often the idle-eviction goroutine
	// scans the pool map.
	defaultEvictInterval = 1 * time.Minute
)

// managedPool wraps a tenant's pgxpool with last-used tracking for
// idle eviction. lastUsed is unix-nano stored atomically so ForTenant
// can update it under a read lock without contending writers.
type managedPool struct {
	pool     *pgxpool.Pool
	lastUsed atomic.Int64
}

// PoolManager owns one *pgxpool.Pool per tenant id, lazily created on
// first ForTenant call and reaped after idle. All tenants share a
// baseConfig (host/port/credentials/SSL); only `Database` and
// `MaxConns` are mutated per tenant onto a copy.
//
// The manager is safe for concurrent use. Eviction runs in a single
// background goroutine, started by Start and stopped by Close (or
// parent ctx cancellation).
type PoolManager struct {
	baseConfig        *pgxpool.Config
	maxConnsPerTenant int32
	idleTimeout       time.Duration
	evictInterval     time.Duration

	mu    sync.RWMutex
	pools map[string]*managedPool

	// started + closed guard Start/Close lifecycle.
	started bool
	closed  bool

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Option configures a PoolManager.
type Option func(*PoolManager)

// WithMaxConns sets the per-tenant pgxpool MaxConns. Default is 2.
func WithMaxConns(n int32) Option {
	return func(m *PoolManager) {
		if n > 0 {
			m.maxConnsPerTenant = n
		}
	}
}

// WithIdleTimeout sets how long a tenant pool may sit idle before
// being evicted. Default is 10 minutes.
func WithIdleTimeout(d time.Duration) Option {
	return func(m *PoolManager) {
		if d > 0 {
			m.idleTimeout = d
		}
	}
}

// WithEvictInterval sets how often the eviction goroutine scans for
// idle pools. Default is 1 minute.
func WithEvictInterval(d time.Duration) Option {
	return func(m *PoolManager) {
		if d > 0 {
			m.evictInterval = d
		}
	}
}

// NewPoolManager constructs a PoolManager. The caller is responsible
// for parsing the base DSN via `pgxpool.ParseConfig`; this matches the
// pattern in db.go where the application pre-parses config and passes
// a ready-to-use *pgxpool.Config. The Database field on baseConfig is
// ignored (overwritten per tenant).
func NewPoolManager(baseConfig *pgxpool.Config, opts ...Option) *PoolManager {
	m := &PoolManager{
		baseConfig:        baseConfig,
		maxConnsPerTenant: defaultMaxConnsPerTenant,
		idleTimeout:       defaultIdleTimeout,
		evictInterval:     defaultEvictInterval,
		pools:             make(map[string]*managedPool),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start launches the idle-eviction goroutine. Calling Start more than
// once is a no-op. Eviction stops when ctx is cancelled or Close is
// called.
func (m *PoolManager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started || m.closed {
		m.mu.Unlock()
		return
	}
	m.started = true
	evictCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	m.wg.Add(1)
	go m.evictLoop(evictCtx)
}

// ForTenant returns the pgxpool for the given tenant, creating it on
// first request. tenantID is a UUID string ("xxxxxxxx-xxxx-..."); it
// is parsed and re-rendered to the canonical lowercase form before
// derivation.
func (m *PoolManager) ForTenant(ctx context.Context, tenantID string) (*pgxpool.Pool, error) {
	if m == nil {
		return nil, errors.New("pool manager is nil")
	}

	dbName, err := tenantDBName(tenantID)
	if err != nil {
		return nil, err
	}

	// Fast path: pool exists. Bump lastUsed under a read lock.
	m.mu.RLock()
	closed := m.closed
	if !closed {
		if mp, ok := m.pools[tenantID]; ok {
			mp.lastUsed.Store(time.Now().UnixNano())
			pool := mp.pool
			m.mu.RUnlock()
			return pool, nil
		}
	}
	m.mu.RUnlock()
	if closed {
		return nil, errors.New("pool manager is closed")
	}

	// Slow path: create pool. Parse-side mutation requires a writer
	// lock; double-check under the lock in case another goroutine
	// raced ahead.
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("pool manager is closed")
	}
	if mp, ok := m.pools[tenantID]; ok {
		mp.lastUsed.Store(time.Now().UnixNano())
		pool := mp.pool
		m.mu.Unlock()
		return pool, nil
	}

	cfg := m.baseConfig.Copy()
	cfg.ConnConfig.Database = dbName
	cfg.MaxConns = m.maxConnsPerTenant

	// pgxpool.NewWithConfig takes ownership of cfg; we mustn't reuse
	// it, hence the Copy() above.
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("create pool for tenant %s (db=%s): %w", tenantID, dbName, err)
	}

	mp := &managedPool{pool: pool}
	mp.lastUsed.Store(time.Now().UnixNano())
	m.pools[tenantID] = mp
	m.mu.Unlock()
	return pool, nil
}

// Close stops eviction, closes every cached pool, and clears the map.
// Safe to call from any goroutine; subsequent ForTenant calls return
// an error. Always returns nil today; the signature returns error so
// future implementations can surface partial-close failures without a
// breaking change.
func (m *PoolManager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	cancel := m.cancel
	pools := m.pools
	m.pools = make(map[string]*managedPool)
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	m.wg.Wait()

	// Close pools outside the lock — pgxpool.Close blocks on in-flight
	// connections and we don't want to stall any other code path that
	// might be racing on the manager.
	for _, mp := range pools {
		mp.pool.Close()
	}
	return nil
}

func (m *PoolManager) evictLoop(ctx context.Context) {
	defer m.wg.Done()
	t := time.NewTicker(m.evictInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.evictIdle(time.Now())
		}
	}
}

// evictIdle closes pools whose lastUsed is older than idleTimeout.
// Identification + map removal happens under the write lock; the
// actual pool.Close() happens AFTER releasing the lock so a slow
// connection drain can't stall ForTenant for other tenants.
func (m *PoolManager) evictIdle(now time.Time) {
	threshold := now.Add(-m.idleTimeout).UnixNano()

	var toClose []*managedPool
	m.mu.Lock()
	for id, mp := range m.pools {
		if mp.lastUsed.Load() <= threshold {
			toClose = append(toClose, mp)
			delete(m.pools, id)
		}
	}
	m.mu.Unlock()

	for _, mp := range toClose {
		mp.pool.Close()
	}
}

// tenantDBName parses a UUID string and returns the canonical
// `tenant_<32hex>` database name. The parsed UUID is rendered through
// uuid.String() (lowercase canonical form) before stripping hyphens,
// so any accepted-by-uuid.Parse spelling normalizes to the regex.
func tenantDBName(tenantID string) (string, error) {
	parsed, err := uuid.Parse(tenantID)
	if err != nil {
		return "", fmt.Errorf("invalid tenant id %q: %w", tenantID, err)
	}
	name := "tenant_" + strings.ReplaceAll(parsed.String(), "-", "")
	if err := validateTenantDBName(name); err != nil {
		// Defensive: uuid.Parse + ReplaceAll should always produce a
		// regex-matching string. If this ever fires, derivation is
		// out of sync with the regex — fail loudly.
		return "", err
	}
	return name, nil
}

// validateTenantDBName enforces the `^tenant_[a-f0-9]{32}$` shape
// before any DDL string-interpolation. The regex matches the schema
// CHECK constraint on `platform.tenants.db_name`.
func validateTenantDBName(name string) error {
	if !dbNameRegex.MatchString(name) {
		return fmt.Errorf("invalid tenant db name: %q", name)
	}
	return nil
}
