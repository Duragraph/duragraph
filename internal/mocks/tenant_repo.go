package mocks

import (
	"context"
	"sort"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// TenantRepository is an in-memory mock of tenant.Repository for
// command handler / subscriber unit tests.
type TenantRepository struct {
	mu      sync.RWMutex
	Tenants map[string]*tenant.Tenant

	// Index by user_id for GetByUserID (1:1 with tenant.id).
	tenantsByUser map[string]*tenant.Tenant

	SaveFunc         func(ctx context.Context, t *tenant.Tenant) error
	GetByIDFunc      func(ctx context.Context, id string) (*tenant.Tenant, error)
	GetByUserIDFunc  func(ctx context.Context, userID string) (*tenant.Tenant, error)
	ListByStatusFunc func(ctx context.Context, status tenant.Status, limit, offset int) ([]*tenant.Tenant, error)
}

// NewTenantRepository constructs an empty in-memory TenantRepository
// mock.
func NewTenantRepository() *TenantRepository {
	return &TenantRepository{
		Tenants:       make(map[string]*tenant.Tenant),
		tenantsByUser: make(map[string]*tenant.Tenant),
	}
}

// Save stores t under both ID and user_id indexes. Func override takes
// precedence. Mirrors the postgres repository's contract of clearing
// emitted domain events after persistence.
func (m *TenantRepository) Save(ctx context.Context, t *tenant.Tenant) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, t)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Tenants[t.ID()] = t
	m.tenantsByUser[t.UserID()] = t
	t.ClearEvents()
	return nil
}

// GetByID returns the tenant with the given ID or NotFound.
func (m *TenantRepository) GetByID(ctx context.Context, id string) (*tenant.Tenant, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.Tenants[id]
	if !ok {
		return nil, errors.NotFound("tenant", id)
	}
	return t, nil
}

// GetByUserID returns the tenant for the given user_id or NotFound.
func (m *TenantRepository) GetByUserID(ctx context.Context, userID string) (*tenant.Tenant, error) {
	if m.GetByUserIDFunc != nil {
		return m.GetByUserIDFunc(ctx, userID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenantsByUser[userID]
	if !ok {
		return nil, errors.NotFound("tenant for user", userID)
	}
	return t, nil
}

// ListByStatus returns tenants matching status with pagination.
// Results are sorted by CreatedAt ascending (ID as tiebreaker) for
// deterministic behavior — the postgres repo's SELECT uses ORDER BY,
// so callers expect a stable order.
func (m *TenantRepository) ListByStatus(ctx context.Context, status tenant.Status, limit, offset int) ([]*tenant.Tenant, error) {
	if m.ListByStatusFunc != nil {
		return m.ListByStatusFunc(ctx, status, limit, offset)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*tenant.Tenant, 0)
	for _, t := range m.Tenants {
		if t.Status() == status {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt().Equal(out[j].CreatedAt()) {
			return out[i].ID() < out[j].ID()
		}
		return out[i].CreatedAt().Before(out[j].CreatedAt())
	})
	if offset >= len(out) {
		return []*tenant.Tenant{}, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}
