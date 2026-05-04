package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// ResumeUser is the input command for ResumeUserHandler.
//
// Reverses suspension. Only the User aggregate is mutated: the Tenant
// aggregate has no Resume() method by design — the tenant state
// machine treats Suspended as terminal at the aggregate level (see
// internal/domain/tenant/status.go). Per duragraph-spec/auth/oauth.yml
// the user-level resume is what restores access; the tenant DB and
// NATS Account never went away (suspend keeps them intact, gating only
// at the JWT/middleware path). So flipping the User row back to
// approved is sufficient for the next OAuth callback to mint a JWT
// with tenant_id populated again.
//
// Documented asymmetry: User has Resume; Tenant does not. The
// admin UI surfaces only the user-level Resume; there is no "resume
// tenant" endpoint. If a tenant ends up in suspended state with the
// user still approved (operator manually fiddled with the DB, or
// future suspend-tenant-only endpoint added) the only escape today is
// a fresh tenant created via the bootstrap-recovery path of
// ApproveUserHandler — out of scope for v0.
type ResumeUser struct {
	UserID          string
	ResumedByUserID string
}

// ResumeUserHandler handles the ResumeUser command.
type ResumeUserHandler struct {
	userRepo user.Repository
}

// NewResumeUserHandler constructs a ResumeUserHandler.
func NewResumeUserHandler(userRepo user.Repository) *ResumeUserHandler {
	return &ResumeUserHandler{userRepo: userRepo}
}

// Handle transitions the user back to approved. user.Resume is the
// only state-machine guard — it has no self-action guard (deliberate,
// per the comment in internal/domain/user/user.go: a single-admin
// deployment must be able to recover its own suspended account).
//
// Idempotency: a re-issued Resume on an already-approved user is a
// no-op success.
func (h *ResumeUserHandler) Handle(ctx context.Context, cmd ResumeUser) error {
	if cmd.UserID == "" {
		return errors.InvalidInput("user_id", "user_id is required")
	}
	if cmd.ResumedByUserID == "" {
		return errors.InvalidInput("resumed_by_user_id", "resumed_by_user_id is required")
	}

	u, err := h.userRepo.GetByID(ctx, cmd.UserID)
	if err != nil {
		return err
	}

	if u.Status() == user.StatusApproved {
		// Idempotent no-op.
		return nil
	}

	if err := u.Resume(cmd.ResumedByUserID); err != nil {
		return err
	}
	if err := h.userRepo.Save(ctx, u); err != nil {
		return errors.Internal("failed to save user", err)
	}
	return nil
}
