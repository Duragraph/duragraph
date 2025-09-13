package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"runtime/bridge"
)

type Run struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id"`
	Assistant string    `json:"assistant_id"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Completed bool      `json:"completed"`
	LastError string    `json:"error,omitempty"`
}

var runStore = map[string]*Run{}

func postRunHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ThreadID    string `json:"thread_id"`
		AssistantID string `json:"assistant_id"`
		Input       string `json:"input"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	id := uuid.New().String()
	run := &Run{
		ID:        id,
		ThreadID:  body.ThreadID,
		Assistant: body.AssistantID,
		Status:    "started",
		StartedAt: time.Now(),
	}
	runStore[id] = run

	// Call into bridge to start the run (stub for now)
	go bridge.StartRun(id, body.Input)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"run_id": id})
}

func getRunHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	run, ok := runStore[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(run)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		http.Error(w, "missing run_id", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	// Simulate three events
	fmt.Fprintf(w, "event: run_started\ndata: {\"run_id\":\"%s\"}\n\n", runID)
	flusher.Flush()
	time.Sleep(500 * time.Millisecond)

	fmt.Fprintf(w, "event: message_delta\ndata: {\"content\":\"echo\"}\n\n")
	flusher.Flush()
	time.Sleep(500 * time.Millisecond)

	fmt.Fprintf(w, "event: run_completed\ndata: {\"run_id\":\"%s\",\"status\":\"completed\"}\n\n", runID)
	flusher.Flush()

	// Update run status
	if run, ok := runStore[runID]; ok {
		run.Status = "completed"
		run.Completed = true
	}
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/runs", postRunHandler).Methods("POST")
	r.HandleFunc("/runs/{id}", getRunHandler).Methods("GET")
	r.HandleFunc("/stream", streamHandler).Methods("GET")

	addr := ":8080"
	log.Printf("API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
