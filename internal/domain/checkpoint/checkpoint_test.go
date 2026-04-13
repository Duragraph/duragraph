package checkpoint

import (
	"testing"
	"time"
)

func TestNewCheckpoint_Valid(t *testing.T) {
	cv := map[string]interface{}{"messages": []string{"hello"}}
	cp, err := NewCheckpoint("thread-1", "default", "", "", cv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp.ID() == "" {
		t.Error("ID should not be empty")
	}
	if cp.ThreadID() != "thread-1" {
		t.Errorf("expected ThreadID=thread-1, got %s", cp.ThreadID())
	}
	if cp.CheckpointNS() != "default" {
		t.Error("wrong namespace")
	}
	if cp.CheckpointID() == "" {
		t.Error("CheckpointID should be auto-generated when empty")
	}
	if cp.ParentCheckpointID() != "" {
		t.Error("ParentCheckpointID should be empty")
	}
	if cp.ChannelValues()["messages"] == nil {
		t.Error("channel values not set")
	}
	if cp.ChannelVersions() == nil {
		t.Error("ChannelVersions should be initialized")
	}
	if cp.VersionsSeen() == nil {
		t.Error("VersionsSeen should be initialized")
	}
	if cp.PendingSends() == nil {
		t.Error("PendingSends should be initialized")
	}
	if cp.CreatedAt().IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestNewCheckpoint_ExplicitCheckpointID(t *testing.T) {
	cp, err := NewCheckpoint("thread-1", "ns", "explicit-id", "parent-id", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp.CheckpointID() != "explicit-id" {
		t.Errorf("expected explicit-id, got %s", cp.CheckpointID())
	}
	if cp.ParentCheckpointID() != "parent-id" {
		t.Error("parent checkpoint ID not set")
	}
}

func TestNewCheckpoint_EmptyThreadID(t *testing.T) {
	_, err := NewCheckpoint("", "ns", "", "", nil)
	if err == nil {
		t.Fatal("expected error for empty thread_id")
	}
	if err != ErrInvalidThreadID {
		t.Errorf("expected ErrInvalidThreadID, got %v", err)
	}
}

func TestCheckpoint_SetChannelValue(t *testing.T) {
	cp, _ := NewCheckpoint("thread-1", "ns", "", "", map[string]interface{}{})

	cp.SetChannelValue("messages", "hello")
	val, ok := cp.GetChannelValue("messages")
	if !ok {
		t.Error("should find channel value")
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %v", val)
	}

	if cp.ChannelVersions()["messages"] != 1 {
		t.Errorf("expected version=1, got %d", cp.ChannelVersions()["messages"])
	}

	cp.SetChannelValue("messages", "world")
	if cp.ChannelVersions()["messages"] != 2 {
		t.Errorf("expected version=2 after second set, got %d", cp.ChannelVersions()["messages"])
	}
}

func TestCheckpoint_GetChannelValue_NotFound(t *testing.T) {
	cp, _ := NewCheckpoint("thread-1", "ns", "", "", map[string]interface{}{})
	_, ok := cp.GetChannelValue("nonexistent")
	if ok {
		t.Error("should return false for nonexistent channel")
	}
}

func TestCheckpoint_PendingSends(t *testing.T) {
	cp, _ := NewCheckpoint("thread-1", "ns", "", "", nil)

	cp.AddPendingSend(map[string]interface{}{"channel": "out", "value": "a"})
	cp.AddPendingSend(map[string]interface{}{"channel": "out", "value": "b"})

	sends := cp.PendingSends()
	if len(sends) != 2 {
		t.Fatalf("expected 2 pending sends, got %d", len(sends))
	}

	cp.ClearPendingSends()
	if len(cp.PendingSends()) != 0 {
		t.Error("pending sends should be empty after clear")
	}
}

func TestReconstitute(t *testing.T) {
	now := time.Now()
	cv := map[string]interface{}{"k": "v"}
	cver := map[string]int{"k": 3}
	vs := map[string]map[string]int{"node1": {"k": 2}}
	ps := []map[string]interface{}{{"channel": "out"}}

	cp := Reconstitute("id-1", "thread-1", "ns", "cp-1", "parent-1", cv, cver, vs, ps, now)

	if cp.ID() != "id-1" {
		t.Error("wrong ID")
	}
	if cp.ThreadID() != "thread-1" {
		t.Error("wrong ThreadID")
	}
	if cp.CheckpointNS() != "ns" {
		t.Error("wrong CheckpointNS")
	}
	if cp.CheckpointID() != "cp-1" {
		t.Error("wrong CheckpointID")
	}
	if cp.ParentCheckpointID() != "parent-1" {
		t.Error("wrong ParentCheckpointID")
	}
	if cp.ChannelValues()["k"] != "v" {
		t.Error("wrong ChannelValues")
	}
	if cp.ChannelVersions()["k"] != 3 {
		t.Error("wrong ChannelVersions")
	}
	if cp.VersionsSeen()["node1"]["k"] != 2 {
		t.Error("wrong VersionsSeen")
	}
	if len(cp.PendingSends()) != 1 {
		t.Error("wrong PendingSends")
	}
	if !cp.CreatedAt().Equal(now) {
		t.Error("wrong CreatedAt")
	}
}

func TestErrors(t *testing.T) {
	if ErrInvalidThreadID.Error() != "thread_id is required" {
		t.Error("wrong ErrInvalidThreadID message")
	}
	if ErrCheckpointNotFound.Error() != "checkpoint not found" {
		t.Error("wrong ErrCheckpointNotFound message")
	}
}

func TestNewCheckpointWrite(t *testing.T) {
	blob := map[string]interface{}{"data": "test"}
	w := NewCheckpointWrite("thread-1", "ns", "cp-1", "task-1", 0, "messages", "put", blob)

	if w.ID() == "" {
		t.Error("ID should not be empty")
	}
	if w.ThreadID() != "thread-1" {
		t.Error("wrong ThreadID")
	}
	if w.CheckpointNS() != "ns" {
		t.Error("wrong CheckpointNS")
	}
	if w.CheckpointID() != "cp-1" {
		t.Error("wrong CheckpointID")
	}
	if w.TaskID() != "task-1" {
		t.Error("wrong TaskID")
	}
	if w.Idx() != 0 {
		t.Errorf("expected Idx=0, got %d", w.Idx())
	}
	if w.Channel() != "messages" {
		t.Error("wrong Channel")
	}
	if w.WriteType() != "put" {
		t.Error("wrong WriteType")
	}
	if w.Blob()["data"] != "test" {
		t.Error("wrong Blob")
	}
	if w.CreatedAt().IsZero() {
		t.Error("CreatedAt should be set")
	}
}
