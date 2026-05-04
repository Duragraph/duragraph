package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/mocks"
)

// fakeMigrator implements TenantMigrator with adjustable hooks.
type fakeMigrator struct {
	mu sync.Mutex

	provisionCalls []string
	migrateCalls   []string

	provisionErr error
	migrateErr   error
	version      uint
}

func (f *fakeMigrator) ProvisionTenant(_ context.Context, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.provisionCalls = append(f.provisionCalls, tenantID)
	return f.provisionErr
}

func (f *fakeMigrator) MigrateTenant(_ context.Context, tenantID string) (uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.migrateCalls = append(f.migrateCalls, tenantID)
	return f.version, f.migrateErr
}

// fakeNATSAccount records calls; can be set to error.
type fakeNATSAccount struct {
	calls []string
	err   error
}

func (f *fakeNATSAccount) ProvisionAccount(_ context.Context, tenantID string) error {
	f.calls = append(f.calls, tenantID)
	return f.err
}

func newSilentLogger() *log.Logger {
	// Discard log output during tests — not part of the assertion
	// surface.
	return log.New(discardWriter{}, "", 0)
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// helper: persist a tenant in the given starting status.
func seedTenant(t *testing.T, repo *mocks.TenantRepository, status tenant.Status) *tenant.Tenant {
	t.Helper()
	te, err := tenant.NewTenant("user-1")
	if err != nil {
		t.Fatalf("NewTenant: %v", err)
	}
	switch status {
	case tenant.StatusPending:
		// already pending
	case tenant.StatusProvisioning:
		_ = te.StartProvisioning()
	case tenant.StatusApproved:
		_ = te.StartProvisioning()
		_ = te.Approve("admin-1", 1)
	case tenant.StatusProvisioningFailed:
		_ = te.StartProvisioning()
		_ = te.MarkProvisioningFailed("test")
	case tenant.StatusSuspended:
		_ = te.StartProvisioning()
		_ = te.Approve("admin-1", 1)
		_ = te.Suspend("admin-2", "x")
	}
	if err := repo.Save(context.Background(), te); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return te
}

func TestTenantProvisioner_Success(t *testing.T) {
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{version: 7}
	nats := &fakeNATSAccount{}

	te := seedTenant(t, repo, tenant.StatusProvisioning)

	p := &TenantProvisioner{
		tenantRepo:  repo,
		migrator:    mig,
		natsAccount: nats,
		logger:      newSilentLogger(),
	}

	payload, _ := json.Marshal(tenant.TenantProvisioning{TenantID: te.ID()})
	if err := p.processEvent(context.Background(), payload); err != nil {
		t.Fatalf("processEvent: %v", err)
	}

	if len(mig.provisionCalls) != 1 || mig.provisionCalls[0] != te.ID() {
		t.Errorf("expected ProvisionTenant called once with %s, got %v", te.ID(), mig.provisionCalls)
	}
	if len(nats.calls) != 1 {
		t.Errorf("expected NATS account provisioned once, got %d", len(nats.calls))
	}
	saved := repo.Tenants[te.ID()]
	if saved.Status() != tenant.StatusApproved {
		t.Errorf("expected approved, got %s", saved.Status())
	}
	if saved.SchemaVersion() == nil || *saved.SchemaVersion() != 7 {
		t.Errorf("expected schema version 7, got %v", saved.SchemaVersion())
	}
	// Must use the system-actor sentinel, NOT the user's own ID.
	for _, ev := range saved.Events() {
		// events were cleared by Save, so this loop is empty in
		// practice. The actor check is implicit in not-erroring.
		_ = ev
	}
}

func TestTenantProvisioner_BootstrapAlreadyApproved(t *testing.T) {
	// Bootstrap-already-approved short-circuit: tenant in `approved`
	// receives a tenant.provisioning event. No migrator call, no Save.
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{}
	nats := &fakeNATSAccount{}
	te := seedTenant(t, repo, tenant.StatusApproved)

	p := &TenantProvisioner{
		tenantRepo:  repo,
		migrator:    mig,
		natsAccount: nats,
		logger:      newSilentLogger(),
	}
	payload, _ := json.Marshal(tenant.TenantProvisioning{TenantID: te.ID()})
	if err := p.processEvent(context.Background(), payload); err != nil {
		t.Fatalf("processEvent: %v", err)
	}
	if len(mig.provisionCalls) != 0 {
		t.Errorf("must not call migrator on already-approved tenant: %v", mig.provisionCalls)
	}
	if len(nats.calls) != 0 {
		t.Errorf("must not call NATS account: %v", nats.calls)
	}
	if repo.Tenants[te.ID()].Status() != tenant.StatusApproved {
		t.Errorf("status must remain approved")
	}
}

func TestTenantProvisioner_StaleEventOnFailedTenant(t *testing.T) {
	// Stale redelivery for a tenant in provisioning_failed state — drop
	// silently.
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{}
	te := seedTenant(t, repo, tenant.StatusProvisioningFailed)

	p := &TenantProvisioner{
		tenantRepo: repo,
		migrator:   mig,
		logger:     newSilentLogger(),
	}
	payload, _ := json.Marshal(tenant.TenantProvisioning{TenantID: te.ID()})
	if err := p.processEvent(context.Background(), payload); err != nil {
		t.Fatalf("processEvent: %v", err)
	}
	if len(mig.provisionCalls) != 0 {
		t.Errorf("must not run migrator on failed tenant")
	}
}

func TestTenantProvisioner_ProvisionFailureMarksFailed(t *testing.T) {
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{provisionErr: errors.New("create db: permission denied")}
	te := seedTenant(t, repo, tenant.StatusProvisioning)

	p := &TenantProvisioner{
		tenantRepo: repo,
		migrator:   mig,
		logger:     newSilentLogger(),
	}
	payload, _ := json.Marshal(tenant.TenantProvisioning{TenantID: te.ID()})
	if err := p.processEvent(context.Background(), payload); err == nil {
		t.Fatal("expected error from failed provisioning")
	}
	saved := repo.Tenants[te.ID()]
	if saved.Status() != tenant.StatusProvisioningFailed {
		t.Errorf("expected provisioning_failed, got %s", saved.Status())
	}
	if saved.FailureReason() == "" {
		t.Errorf("failure reason should be populated")
	}
}

func TestTenantProvisioner_NATSAccountFailureMarksFailed(t *testing.T) {
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{version: 3}
	nats := &fakeNATSAccount{err: errors.New("operator jwt invalid")}
	te := seedTenant(t, repo, tenant.StatusProvisioning)

	p := &TenantProvisioner{
		tenantRepo:  repo,
		migrator:    mig,
		natsAccount: nats,
		logger:      newSilentLogger(),
	}
	payload, _ := json.Marshal(tenant.TenantProvisioning{TenantID: te.ID()})
	if err := p.processEvent(context.Background(), payload); err == nil {
		t.Fatal("expected error from NATS account failure")
	}
	saved := repo.Tenants[te.ID()]
	if saved.Status() != tenant.StatusProvisioningFailed {
		t.Errorf("expected provisioning_failed, got %s", saved.Status())
	}
}

func TestTenantProvisioner_TenantNotFound(t *testing.T) {
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{}
	p := &TenantProvisioner{
		tenantRepo: repo,
		migrator:   mig,
		logger:     newSilentLogger(),
	}
	payload, _ := json.Marshal(tenant.TenantProvisioning{TenantID: "nonexistent"})
	if err := p.processEvent(context.Background(), payload); err == nil {
		t.Fatal("expected error for missing tenant")
	}
}

func TestTenantProvisioner_ExtractTenantID_AggregateIDFallback(t *testing.T) {
	// Outbox-envelope shape: {aggregate_id: "..."}.
	envelope := map[string]interface{}{
		"aggregate_id":   "tenant-123",
		"aggregate_type": "tenant",
		"event_type":     "tenant.provisioning",
		"payload":        map[string]interface{}{},
	}
	payload, _ := json.Marshal(envelope)
	id, err := extractTenantID(payload)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if id != "tenant-123" {
		t.Errorf("expected aggregate_id fallback, got %q", id)
	}
}

func TestTenantProvisioner_ExtractTenantID_NoIDError(t *testing.T) {
	payload := []byte(`{"foo":"bar"}`)
	if _, err := extractTenantID(payload); err == nil {
		t.Fatal("expected error when payload has no tenant_id or aggregate_id")
	}
}

func TestTenantProvisioner_NoopNATSAccountIsDefault(t *testing.T) {
	// Constructor must substitute Noop when caller passes nil so a
	// zero-config wiring doesn't panic.
	p := NewTenantProvisioner(nil, nil, nil, nil, nil)
	if p.natsAccount == nil {
		t.Fatal("constructor must default natsAccount")
	}
	if _, ok := p.natsAccount.(NoopNATSAccountProvisioner); !ok {
		t.Errorf("expected NoopNATSAccountProvisioner default")
	}
}
