package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// SuspendUser is the input command for SuspendUserHandler.
//
// Suspends an already-approved user. Per platform.yaml the tenant DB
// and NATS Account remain intact; only API access is gated off via the
// JWT/middleware path. Reversible via Resume.
type SuspendUser struct {
	UserID            string
	SuspendedByUserID string
	Reason            string
}

// SuspendUserHandler handles the SuspendUser command.
//
// Touches BOTH aggregates: User → suspended (state machine guard
// approved → suspended in user.Suspend), Tenant → suspended (state
// machine guard approved → suspended in tenant.Suspend). The tenant
// state machine treats Suspended as terminal at the aggregate level —
// see the comment block at the top of resume_user.go for the
// asymmetry note that drops out of this design.
type SuspendUserHandler struct {
	userRepo   user.Repository
	tenantRepo tenant.Repository
}

// NewSuspendUserHandler constructs a SuspendUserHandler.
func NewSuspendUserHandler(userRepo user.Repository, tenantRepo tenant.Repository) *SuspendUserHandler {
	return &SuspendUserHandler{userRepo: userRepo, tenantRepo: tenantRepo}
}

// Handle suspends the user and (best-effort) the tenant.
//
// Idempotency: a re-issued Suspend on an already-suspended user is a
// no-op success — the OAuth callback's suspended branch and the admin
// UI both rely on idempotent Suspend so a double-click can't surface a
// 400 to the operator. This is enforced ahead of user.Suspend (which
// returns InvalidState on suspended users).
//
// Atomicity: same caveat as ApproveUserHandler — no shared tx between
// User and Tenant Save calls. We save user first, then tenant. If
// tenant.Save fails the user is suspended but the tenant aggregate is
// not. The retry semantics are: re-issuing Suspend is a no-op on the
// user (idempotent short-circuit) but will progress the tenant on the
// retry attempt — admin re-clicks Suspend.
//
// Tenant lookup: a user with no Tenant aggregate (rejection path, or
// bootstrap edge cases) is suspended on the User side only; we do NOT
// 404 on the missing tenant, because the user-side suspension is
// already meaningful (the JWT middleware gates them off).
func (h *SuspendUserHandler) Handle(ctx context.Context, cmd SuspendUser) error {
	if cmd.UserID == "" {
		return errors.InvalidInput("user_id", "user_id is required")
	}
	if cmd.SuspendedByUserID == "" {
		return errors.InvalidInput("suspended_by_user_id", "suspended_by_user_id is required")
	}

	u, err := h.userRepo.GetByID(ctx, cmd.UserID)
	if err != nil {
		return err
	}

	if u.Status() == user.StatusSuspended {
		// Idempotent no-op. Note: user.Status() == suspended could mean
		// "suspended via Reject" (pending → suspended) or "suspended via
		// Suspend" (approved → suspended). We do not attempt to
		// distinguish — both end states are "API access denied", which
		// is what the operator wants out of this endpoint.
		return nil
	}

	if err := u.Suspend(cmd.SuspendedByUserID, cmd.Reason); err != nil {
		return err
	}
	if err := h.userRepo.Save(ctx, u); err != nil {
		return errors.Internal("failed to save user", err)
	}

	// Suspend the tenant if it exists and is approved. Other tenant
	// states (pending / provisioning / failed / suspended) are not
	// transitioned — only approved tenants can be suspended per the
	// tenant state machine. A pending/provisioning tenant left dangling
	// is harmless: the user's JWT path is already gated.
	t, err := h.tenantRepo.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		if errors.Is(err, errors.ErrNotFound) {
			return nil
		}
		return err
	}
	if t.Status() != tenant.StatusApproved {
		return nil
	}
	if err := t.Suspend(cmd.SuspendedByUserID, cmd.Reason); err != nil {
		return err
	}
	if err := h.tenantRepo.Save(ctx, t); err != nil {
		return errors.Internal("failed to save tenant", err)
	}

	return nil
}
