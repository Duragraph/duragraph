package mocks

import (
	"context"
	"sort"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// UserRepository is an in-memory mock of user.Repository for command
// handler unit tests. Each Func override hook lets a test inject
// failure / latency without subclassing.
type UserRepository struct {
	mu    sync.RWMutex
	Users map[string]*user.User

	// Index by (oauth_provider, oauth_id) for GetByOAuth.
	usersByOAuth map[string]*user.User

	SaveFunc          func(ctx context.Context, u *user.User) error
	GetByIDFunc       func(ctx context.Context, id string) (*user.User, error)
	GetByOAuthFunc    func(ctx context.Context, provider, oauthID string) (*user.User, error)
	ListByStatusFunc  func(ctx context.Context, status user.Status, limit, offset int) ([]*user.User, error)
	ListFunc          func(ctx context.Context, status *user.Status, limit, offset int) ([]*user.User, error)
	CountByStatusFunc func(ctx context.Context, status *user.Status) (int, error)
	CountAllFunc      func(ctx context.Context) (int, error)
}

// NewUserRepository constructs an empty in-memory UserRepository mock.
func NewUserRepository() *UserRepository {
	return &UserRepository{
		Users:        make(map[string]*user.User),
		usersByOAuth: make(map[string]*user.User),
	}
}

// Save stores u under both ID and (provider, oauth_id) indexes. Func
// override takes precedence. Mirrors the postgres repository's contract
// of clearing emitted domain events after persistence — without this,
// tests that snapshot Events() at a different point in the flow can
// double-count.
func (m *UserRepository) Save(ctx context.Context, u *user.User) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, u)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Users[u.ID()] = u
	m.usersByOAuth[u.OAuthProvider()+"/"+u.OAuthID()] = u
	u.ClearEvents()
	return nil
}

// GetByID returns the user with the given ID or NotFound.
func (m *UserRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.Users[id]
	if !ok {
		return nil, errors.NotFound("user", id)
	}
	return u, nil
}

// GetByOAuth returns the user with the given (provider, oauth_id) pair.
func (m *UserRepository) GetByOAuth(ctx context.Context, provider, oauthID string) (*user.User, error) {
	if m.GetByOAuthFunc != nil {
		return m.GetByOAuthFunc(ctx, provider, oauthID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.usersByOAuth[provider+"/"+oauthID]
	if !ok {
		return nil, errors.NotFound("user", provider+"/"+oauthID)
	}
	return u, nil
}

// ListByStatus returns users matching status with pagination. Results
// are sorted by CreatedAt ascending (ID as tiebreaker) for
// deterministic behavior — the postgres repo uses ORDER BY in its
// SELECT, so without sorting here a test that compares results
// position-by-position would be flaky against the map iteration order.
func (m *UserRepository) ListByStatus(ctx context.Context, status user.Status, limit, offset int) ([]*user.User, error) {
	if m.ListByStatusFunc != nil {
		return m.ListByStatusFunc(ctx, status, limit, offset)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*user.User, 0)
	for _, u := range m.Users {
		if u.Status() == status {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt().Equal(out[j].CreatedAt()) {
			return out[i].ID() < out[j].ID()
		}
		return out[i].CreatedAt().Before(out[j].CreatedAt())
	})
	if offset >= len(out) {
		return []*user.User{}, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

// CountAll returns the total user count. Used by the bootstrap branch.
func (m *UserRepository) CountAll(ctx context.Context) (int, error) {
	if m.CountAllFunc != nil {
		return m.CountAllFunc(ctx)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.Users), nil
}

// List returns users with optional status filter, sorted by CreatedAt
// ascending (ID as tiebreaker) for deterministic behavior. Mirrors the
// postgres repo's ORDER BY semantics.
func (m *UserRepository) List(ctx context.Context, status *user.Status, limit, offset int) ([]*user.User, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, status, limit, offset)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*user.User, 0)
	for _, u := range m.Users {
		if status != nil && u.Status() != *status {
			continue
		}
		out = append(out, u)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt().Equal(out[j].CreatedAt()) {
			return out[i].ID() < out[j].ID()
		}
		return out[i].CreatedAt().Before(out[j].CreatedAt())
	})
	if offset >= len(out) {
		return []*user.User{}, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

// CountByStatus returns the count matching the given status, or all
// users when status is nil.
func (m *UserRepository) CountByStatus(ctx context.Context, status *user.Status) (int, error) {
	if m.CountByStatusFunc != nil {
		return m.CountByStatusFunc(ctx, status)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if status == nil {
		return len(m.Users), nil
	}
	n := 0
	for _, u := range m.Users {
		if u.Status() == *status {
			n++
		}
	}
	return n, nil
}
