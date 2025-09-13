package bridge

import (
	"log"
	"time"

	"app/runtime/translator"
)

// StartRun simulates execution of a workflow run using translator.HardcodedWorkflow.
func StartRun(runID string, input string) {
	log.Printf("[bridge] Starting run %s with input: %s", runID, input)

	steps := translator.HardcodedWorkflow(input)
	for _, step := range steps {
		log.Printf("[bridge] run %s step: %s", runID, step)
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("[bridge] Completed run %s", runID)
}

// QueryRun returns the run's current state (stubbed as 'completed').
func QueryRun(runID string) map[string]string {
	return map[string]string{
		"id":     runID,
		"status": "completed",
	}
}
