package endpoints

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWorkerRowToAPI(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	r := workerRow{Status: "online", ActiveRuns: 2, Capacity: 4, LeaseExpiresAt: &now}
	got := r.toAPI()
	if got.Status != "online" {
		t.Errorf("status: want online, got %q", got.Status)
	}
}

func TestSnapshotRowToAPI(t *testing.T) {
	r := snapshotRow{ID: 9, AggregateID: mustUUID("11111111-1111-1111-1111-111111111111"), Version: 2, State: []byte(`{"count":2}`)}
	got := r.toAPI()
	if got.CheckpointID != 9 || got.Version != 2 {
		t.Errorf("checkpoint id/version: got %d/%d", got.CheckpointID, got.Version)
	}
	if string(got.State) != `{"count":2}` {
		t.Errorf("state: got %s", got.State)
	}
}

func mustUUID(s string) uuid.UUID { u, _ := uuid.Parse(s); return u }
