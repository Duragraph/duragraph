package endpoints

import (
	"testing"
	"time"
)

func TestStoreItemRowToAPI(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	r := storeItemRow{
		ID:        7,
		Namespace: []string{"users", "42"},
		Key:       "profile",
		Value:     []byte(`{"name":"ada"}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
	got := r.toAPI()
	if got.Key != "profile" {
		t.Errorf("key: want profile, got %q", got.Key)
	}
	if len(got.Namespace) != 2 || got.Namespace[0] != "users" || got.Namespace[1] != "42" {
		t.Errorf("namespace: want [users 42], got %v", got.Namespace)
	}
	if got.Value["name"] != "ada" {
		t.Errorf("value.name: want ada, got %v", got.Value["name"])
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Errorf("timestamps not mapped: %v / %v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestStoreItemRowToAPIEmptyValue(t *testing.T) {
	r := storeItemRow{Namespace: []string{"a"}, Key: "k"}
	got := r.toAPI()
	if got.Value == nil {
		t.Fatal("value should be non-nil empty map, got nil")
	}
	if len(got.Value) != 0 {
		t.Errorf("value: want empty, got %v", got.Value)
	}
}
