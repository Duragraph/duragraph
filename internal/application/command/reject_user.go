package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// RejectUser is the input command for RejectUserHandler.
//
// Rejects a still-pending user. Distinct from Suspend (which applies
// to an already-approved user) — Reject emits user.rejected, never
// touches the Tenant aggregate, and never provisions a tenant DB.
// See duragraph-spec/api/platform.yaml § /api/admin/users/{user_id}/reject.
type RejectUser struct {
	UserID           string
	RejectedByUserID string
	Reason           string
}

// RejectUserHandler handles the RejectUser command.
type RejectUserHandler struct {
	userRepo user.Repository
}

// NewRejectUserHandler constructs a RejectUserHandler.
func NewRejectUserHandler(userRepo user.Repository) *RejectUserHandler {
	return &RejectUserHandler{userRepo: userRepo}
}

// Handle rejects the user. Self-rejection blocked by user.Reject.
//
// No tenant action: per the user.Reject docstring, rejection bypasses
// tenant provisioning entirely. There is no Tenant aggregate to mutate
// for a never-approved user (the bootstrap path is the only branch that
// creates a Tenant aggregate before approval, and bootstrapped users
// don't pass through Reject).
func (h *RejectUserHandler) Handle(ctx context.Context, cmd RejectUser) error {
	if cmd.UserID == "" {
		return errors.InvalidInput("user_id", "user_id is required")
	}
	if cmd.RejectedByUserID == "" {
		return errors.InvalidInput("rejected_by_user_id", "rejected_by_user_id is required")
	}

	u, err := h.userRepo.GetByID(ctx, cmd.UserID)
	if err != nil {
		return err
	}

	if err := u.Reject(cmd.RejectedByUserID, cmd.Reason); err != nil {
		return err
	}

	if err := h.userRepo.Save(ctx, u); err != nil {
		return errors.Internal("failed to save user", err)
	}

	return nil
}
