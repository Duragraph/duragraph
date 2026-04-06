package worker

import (
	"testing"
	"time"
)

func TestWorker_CanExecute(t *testing.T) {
	tests := []struct {
		name    string
		graphs  []string
		graphID string
		want    bool
	}{
		{"graph in capabilities", []string{"graph-a", "graph-b"}, "graph-a", true},
		{"graph not in capabilities", []string{"graph-a"}, "graph-b", false},
		{"empty capabilities", []string{}, "graph-a", false},
		{"nil capabilities", nil, "graph-a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{
				Capabilities: Capabilities{Graphs: tt.graphs},
			}
			if got := w.CanExecute(tt.graphID); got != tt.want {
				t.Errorf("CanExecute(%q) = %v, want %v", tt.graphID, got, tt.want)
			}
		})
	}
}

func TestWorker_HasCapacity(t *testing.T) {
	tests := []struct {
		name       string
		activeRuns int
		maxRuns    int
		want       bool
	}{
		{"has capacity", 2, 5, true},
		{"at capacity", 5, 5, false},
		{"over capacity", 6, 5, false},
		{"zero active", 0, 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{
				ActiveRuns:   tt.activeRuns,
				Capabilities: Capabilities{MaxConcurrentRuns: tt.maxRuns},
			}
			if got := w.HasCapacity(); got != tt.want {
				t.Errorf("HasCapacity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorker_IsHealthy(t *testing.T) {
	threshold := 60 * time.Second

	tests := []struct {
		name          string
		lastHeartbeat time.Time
		want          bool
	}{
		{"recent heartbeat", time.Now().Add(-10 * time.Second), true},
		{"stale heartbeat", time.Now().Add(-2 * time.Minute), false},
		{"exactly at threshold", time.Now().Add(-threshold), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{LastHeartbeat: tt.lastHeartbeat}
			if got := w.IsHealthy(threshold); got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskStatus_Values(t *testing.T) {
	tests := []struct {
		status TaskStatus
		want   string
	}{
		{TaskStatusPending, "pending"},
		{TaskStatusClaimed, "claimed"},
		{TaskStatusCompleted, "completed"},
		{TaskStatusFailed, "failed"},
		{TaskStatusExpired, "expired"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("TaskStatus = %q, want %q", tt.status, tt.want)
			}
		})
	}
}

func TestTaskAssignment_Fields(t *testing.T) {
	now := time.Now()
	claimed := now.Add(time.Second)
	lease := now.Add(2 * time.Minute)

	ta := &TaskAssignment{
		ID:             1,
		RunID:          "run-1",
		WorkerID:       "worker-1",
		Status:         TaskStatusClaimed,
		GraphID:        "graph-1",
		ThreadID:       "thread-1",
		AssistantID:    "assistant-1",
		Input:          map[string]interface{}{"msg": "hi"},
		Config:         map[string]interface{}{"limit": 10},
		CreatedAt:      now,
		ClaimedAt:      &claimed,
		LeaseExpiresAt: &lease,
		RetryCount:     1,
		MaxRetries:     3,
	}

	if ta.RunID != "run-1" {
		t.Errorf("RunID = %q, want 'run-1'", ta.RunID)
	}
	if ta.Status != TaskStatusClaimed {
		t.Errorf("Status = %q, want 'claimed'", ta.Status)
	}
	if ta.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", ta.RetryCount)
	}
	if ta.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", ta.MaxRetries)
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	w := &Worker{
		ID:           "w-1",
		Name:         "test-worker",
		Status:       StatusReady,
		Capabilities: Capabilities{Graphs: []string{"g-1"}, MaxConcurrentRuns: 5},
	}

	reg.Register(w)

	got, ok := reg.Get("w-1")
	if !ok {
		t.Fatal("worker not found after Register")
	}
	if got.Name != "test-worker" {
		t.Errorf("Name = %q, want 'test-worker'", got.Name)
	}
	if got.RegisteredAt.IsZero() {
		t.Error("RegisteredAt should be set")
	}
	if got.LastHeartbeat.IsZero() {
		t.Error("LastHeartbeat should be set on register")
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("Get should return false for nonexistent worker")
	}
}

func TestRegistry_Deregister(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Worker{ID: "w-1"})

	if !reg.Deregister("w-1") {
		t.Error("Deregister should return true for existing worker")
	}
	if reg.Deregister("w-1") {
		t.Error("Deregister should return false after removal")
	}
	if _, ok := reg.Get("w-1"); ok {
		t.Error("worker should not exist after Deregister")
	}
}

func TestRegistry_Heartbeat(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Worker{ID: "w-1", Status: StatusReady})

	ok := reg.Heartbeat("w-1", StatusRunning, 3, 10, 1)
	if !ok {
		t.Fatal("Heartbeat should return true for existing worker")
	}

	w, _ := reg.Get("w-1")
	if w.Status != StatusRunning {
		t.Errorf("Status = %q, want 'running'", w.Status)
	}
	if w.ActiveRuns != 3 {
		t.Errorf("ActiveRuns = %d, want 3", w.ActiveRuns)
	}
	if w.TotalRuns != 10 {
		t.Errorf("TotalRuns = %d, want 10", w.TotalRuns)
	}
	if w.FailedRuns != 1 {
		t.Errorf("FailedRuns = %d, want 1", w.FailedRuns)
	}

	ok = reg.Heartbeat("nonexistent", StatusReady, 0, 0, 0)
	if ok {
		t.Error("Heartbeat should return false for nonexistent worker")
	}
}

func TestRegistry_FindWorkerForGraph(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Worker{
		ID:           "w-1",
		Capabilities: Capabilities{Graphs: []string{"g-1", "g-2"}, MaxConcurrentRuns: 5},
		ActiveRuns:   2,
	})
	reg.Register(&Worker{
		ID:           "w-2",
		Capabilities: Capabilities{Graphs: []string{"g-2", "g-3"}, MaxConcurrentRuns: 3},
		ActiveRuns:   0,
	})

	threshold := 60 * time.Second

	w := reg.FindWorkerForGraph("g-1", threshold)
	if w == nil || w.ID != "w-1" {
		t.Error("should find w-1 for g-1")
	}

	w = reg.FindWorkerForGraph("g-3", threshold)
	if w == nil || w.ID != "w-2" {
		t.Error("should find w-2 for g-3")
	}

	w = reg.FindWorkerForGraph("g-99", threshold)
	if w != nil {
		t.Error("should return nil for unknown graph")
	}
}

func TestRegistry_FindWorkerForGraph_RespectsCapacity(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Worker{
		ID:           "w-full",
		Capabilities: Capabilities{Graphs: []string{"g-1"}, MaxConcurrentRuns: 2},
		ActiveRuns:   2,
	})

	w := reg.FindWorkerForGraph("g-1", 60*time.Second)
	if w != nil {
		t.Error("should not return worker at full capacity")
	}
}

func TestRegistry_GetHealthyWorkers(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Worker{ID: "w-healthy"})

	stale := &Worker{ID: "w-stale", LastHeartbeat: time.Now().Add(-5 * time.Minute)}
	reg.Register(stale)
	stale.LastHeartbeat = time.Now().Add(-5 * time.Minute)

	healthy := reg.GetHealthyWorkers(60 * time.Second)
	if len(healthy) != 1 {
		t.Errorf("got %d healthy workers, want 1", len(healthy))
		return
	}
	if healthy[0].ID != "w-healthy" {
		t.Errorf("healthy worker ID = %q, want 'w-healthy'", healthy[0].ID)
	}
}

func TestRegistry_GetAllWorkers(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Worker{ID: "w-1"})
	reg.Register(&Worker{ID: "w-2"})

	all := reg.GetAllWorkers()
	if len(all) != 2 {
		t.Errorf("got %d workers, want 2", len(all))
	}
}

func TestRegistry_CleanupStaleWorkers(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Worker{ID: "w-healthy"})

	stale := &Worker{ID: "w-stale"}
	reg.Register(stale)
	stale.LastHeartbeat = time.Now().Add(-5 * time.Minute)

	removed := reg.CleanupStaleWorkers(60 * time.Second)
	if removed != 1 {
		t.Errorf("removed %d, want 1", removed)
	}

	_, ok := reg.Get("w-stale")
	if ok {
		t.Error("stale worker should be removed")
	}
	_, ok = reg.Get("w-healthy")
	if !ok {
		t.Error("healthy worker should remain")
	}
}

func TestRegistry_GetGraphDefinition(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Worker{
		ID: "w-1",
		GraphDefinitions: []GraphDefinition{
			{GraphID: "g-1", Name: "Graph One", EntryPoint: "start"},
			{GraphID: "g-2", Name: "Graph Two", EntryPoint: "begin"},
		},
	})

	gd, ok := reg.GetGraphDefinition("g-1")
	if !ok {
		t.Fatal("should find graph definition g-1")
	}
	if gd.Name != "Graph One" {
		t.Errorf("Name = %q, want 'Graph One'", gd.Name)
	}
	if gd.EntryPoint != "start" {
		t.Errorf("EntryPoint = %q, want 'start'", gd.EntryPoint)
	}

	_, ok = reg.GetGraphDefinition("g-99")
	if ok {
		t.Error("should not find nonexistent graph definition")
	}
}
