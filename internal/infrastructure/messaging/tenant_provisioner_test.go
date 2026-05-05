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
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

// fakeMigrator implements TenantMigrator with adjustable hooks.
type fakeMigrator struct {
	mu sync.Mutex

	provisionCalls []string

	provisionErr error
	version      uint
}

func (f *fakeMigrator) ProvisionTenantWithVersion(_ context.Context, tenantID string) (uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.provisionCalls = append(f.provisionCalls, tenantID)
	if f.provisionErr != nil {
		return 0, f.provisionErr
	}
	return f.version, nil
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

// helper: persist a tenant in the given starting status. Setup
// transitions are checked with t.Fatalf so a state-machine guard
// regression doesn't silently produce a tenant in the wrong state.
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
		if err := te.StartProvisioning(); err != nil {
			t.Fatalf("seed StartProvisioning: %v", err)
		}
	case tenant.StatusApproved:
		if err := te.StartProvisioning(); err != nil {
			t.Fatalf("seed StartProvisioning: %v", err)
		}
		if err := te.Approve("admin-1", 1); err != nil {
			t.Fatalf("seed Approve: %v", err)
		}
	case tenant.StatusProvisioningFailed:
		if err := te.StartProvisioning(); err != nil {
			t.Fatalf("seed StartProvisioning: %v", err)
		}
		if err := te.MarkProvisioningFailed("test"); err != nil {
			t.Fatalf("seed MarkProvisioningFailed: %v", err)
		}
	case tenant.StatusSuspended:
		if err := te.StartProvisioning(); err != nil {
			t.Fatalf("seed StartProvisioning: %v", err)
		}
		if err := te.Approve("admin-1", 1); err != nil {
			t.Fatalf("seed Approve: %v", err)
		}
		if err := te.Suspend("admin-2", "x"); err != nil {
			t.Fatalf("seed Suspend: %v", err)
		}
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

	// Capture the events emitted by the SECOND Save (the
	// approval). The seed's Save was already cleared by the mock's
	// ClearEvents contract, so we install the SaveFunc only after
	// seeding — otherwise we'd snapshot the seed's
	// (provisioning_started) events, not the (approved) events we
	// want to inspect.
	var capturedEvents []eventbus.Event
	repo.SaveFunc = func(_ context.Context, te *tenant.Tenant) error {
		// Snapshot the events on this tenant BEFORE we clear them.
		for _, ev := range te.Events() {
			capturedEvents = append(capturedEvents, ev)
		}
		// Persist + clear, mirroring the default mock behavior.
		repo.Tenants[te.ID()] = te
		te.ClearEvents()
		return nil
	}

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
		t.Errorf("expected ProvisionTenantWithVersion called once with %s, got %v", te.ID(), mig.provisionCalls)
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

	// Find the tenant.approved event; assert the actor is the
	// system-actor sentinel (NOT the user's own ID). This protects
	// the documented design choice that the subscriber's terminal
	// transition is performed as the platform itself, not as any
	// human admin.
	var approved *tenant.TenantApproved
	for _, ev := range capturedEvents {
		if a, ok := ev.(tenant.TenantApproved); ok {
			a := a
			approved = &a
			break
		}
	}
	if approved == nil {
		var types []string
		for _, ev := range capturedEvents {
			types = append(types, ev.EventType())
		}
		t.Fatalf("expected tenant.approved event captured; got types=%v", types)
	}
	if approved.ApprovedByUserID != tenant.SystemActorUserID {
		t.Errorf("ApprovedByUserID = %q, want SystemActorUserID %q",
			approved.ApprovedByUserID, tenant.SystemActorUserID)
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
	// Terminal-failure-already-persisted: processEvent returns nil
	// (so handleMessage Acks; redelivery would just no-op against
	// the now-failed tenant). Verification is on the persisted
	// state, not the return value.
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{provisionErr: errors.New("create db: permission denied")}
	te := seedTenant(t, repo, tenant.StatusProvisioning)

	p := &TenantProvisioner{
		tenantRepo: repo,
		migrator:   mig,
		logger:     newSilentLogger(),
	}
	payload, _ := json.Marshal(tenant.TenantProvisioning{TenantID: te.ID()})
	if err := p.processEvent(context.Background(), payload); err != nil {
		t.Fatalf("terminal failure should return nil (already persisted): %v", err)
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
	if err := p.processEvent(context.Background(), payload); err != nil {
		t.Fatalf("terminal failure should return nil (already persisted): %v", err)
	}
	saved := repo.Tenants[te.ID()]
	if saved.Status() != tenant.StatusProvisioningFailed {
		t.Errorf("expected provisioning_failed, got %s", saved.Status())
	}
}

func TestTenantProvisioner_TenantNotFound(t *testing.T) {
	// NotFound is non-retryable but distinct from a transient blip.
	// processEvent wraps with errTenantNotFound so handleMessage
	// classifies it as Term-able. Test asserts the sentinel.
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{}
	p := &TenantProvisioner{
		tenantRepo: repo,
		migrator:   mig,
		logger:     newSilentLogger(),
	}
	payload, _ := json.Marshal(tenant.TenantProvisioning{TenantID: "nonexistent"})
	err := p.processEvent(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error for missing tenant")
	}
	if !errors.Is(err, errTenantNotFound) {
		t.Errorf("expected errTenantNotFound sentinel, got %v", err)
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

func TestTenantProvisioner_MalformedPayloadIsTermable(t *testing.T) {
	// processEvent on garbage bytes should return an error wrapping
	// errMalformedPayload so handleMessage routes it to Term (no
	// further redelivery from JetStream).
	repo := mocks.NewTenantRepository()
	mig := &fakeMigrator{}
	p := &TenantProvisioner{
		tenantRepo: repo,
		migrator:   mig,
		logger:     newSilentLogger(),
	}
	err := p.processEvent(context.Background(), []byte(`not-json`))
	if err == nil {
		t.Fatal("expected error from malformed payload")
	}
	if !errors.Is(err, errMalformedPayload) {
		t.Errorf("expected errMalformedPayload sentinel, got %v", err)
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
