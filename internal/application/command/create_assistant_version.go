package command

import (
	"context"
	"time"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/google/uuid"
)

// CreateAssistantVersionCommand contains data for creating a new assistant version
type CreateAssistantVersionCommand struct {
	AssistantID string
	GraphID     string
	Config      map[string]interface{}
	Context     []interface{}
}

// CreateAssistantVersionHandler handles creating new assistant versions
type CreateAssistantVersionHandler struct {
	repository workflow.AssistantRepository
}

// NewCreateAssistantVersionHandler creates a new handler
func NewCreateAssistantVersionHandler(repository workflow.AssistantRepository) *CreateAssistantVersionHandler {
	return &CreateAssistantVersionHandler{
		repository: repository,
	}
}

// Handle creates a new version of an assistant
func (h *CreateAssistantVersionHandler) Handle(ctx context.Context, cmd CreateAssistantVersionCommand) (*workflow.AssistantVersionInfo, error) {
	// Get the current versions to determine next version number
	versions, err := h.repository.FindVersions(ctx, cmd.AssistantID, 1)
	if err != nil {
		return nil, err
	}

	nextVersion := 1
	if len(versions) > 0 {
		nextVersion = versions[0].Version + 1
	}

	// Create the new version
	version := workflow.AssistantVersionInfo{
		ID:          uuid.New().String(),
		AssistantID: cmd.AssistantID,
		Version:     nextVersion,
		GraphID:     cmd.GraphID,
		Config:      cmd.Config,
		Context:     cmd.Context,
		CreatedAt:   time.Now(),
	}

	if err := h.repository.SaveVersion(ctx, version); err != nil {
		return nil, err
	}

	return &version, nil
}

// SetLatestVersionCommand contains data for setting the latest assistant version
type SetLatestVersionCommand struct {
	AssistantID string
	Version     int
}

// SetLatestVersionHandler handles setting the latest assistant version
type SetLatestVersionHandler struct {
	repository workflow.AssistantRepository
}

// NewSetLatestVersionHandler creates a new handler
func NewSetLatestVersionHandler(repository workflow.AssistantRepository) *SetLatestVersionHandler {
	return &SetLatestVersionHandler{
		repository: repository,
	}
}

// Handle sets the latest version of an assistant
func (h *SetLatestVersionHandler) Handle(ctx context.Context, cmd SetLatestVersionCommand) error {
	return h.repository.SetLatestVersion(ctx, cmd.AssistantID, cmd.Version)
}
