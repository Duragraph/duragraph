package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/humanloop"
	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// SubmitToolOutputs command
type SubmitToolOutputs struct {
	RunID       string
	ToolOutputs []map[string]interface{}
}

// SubmitToolOutputsHandler handles the SubmitToolOutputs command
type SubmitToolOutputsHandler struct {
	runRepo       run.Repository
	interruptRepo humanloop.Repository
}

// NewSubmitToolOutputsHandler creates a new SubmitToolOutputsHandler
func NewSubmitToolOutputsHandler(runRepo run.Repository, interruptRepo humanloop.Repository) *SubmitToolOutputsHandler {
	return &SubmitToolOutputsHandler{
		runRepo:       runRepo,
		interruptRepo: interruptRepo,
	}
}

// Handle handles the SubmitToolOutputs command
func (h *SubmitToolOutputsHandler) Handle(ctx context.Context, cmd SubmitToolOutputs) error {
	// Load run
	runAgg, err := h.runRepo.FindByID(ctx, cmd.RunID)
	if err != nil {
		return err
	}

	// Check if run requires action
	if runAgg.Status() != run.StatusRequiresAction {
		return errors.InvalidState(runAgg.Status().String(), "submit_tool_outputs")
	}

	// Find unresolved interrupts
	interrupts, err := h.interruptRepo.FindUnresolvedByRunID(ctx, cmd.RunID)
	if err != nil {
		return err
	}

	if len(interrupts) == 0 {
		return errors.InvalidState("no_interrupts", "submit_tool_outputs")
	}

	// Resolve the first interrupt (could be enhanced to resolve all)
	interrupt := interrupts[0]
	if err := interrupt.Resolve(cmd.ToolOutputs); err != nil {
		return err
	}

	// Save interrupt
	if err := h.interruptRepo.Update(ctx, interrupt); err != nil {
		return err
	}

	// Resume run
	if err := runAgg.Resume(interrupt.ID(), cmd.ToolOutputs); err != nil {
		return err
	}

	// Save run
	if err := h.runRepo.Update(ctx, runAgg); err != nil {
		return err
	}

	return nil
}
