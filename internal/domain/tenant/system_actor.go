package tenant

// SystemActorUserID is the sentinel approved_by_user_id passed to
// tenant.Approve when the actor is the platform-provisioner subscriber
// (i.e. the system itself, not a human admin).
//
// Why a sentinel and not the tenant's own user_id?
//
// The platform-provisioner subscriber has no human approver in scope —
// it is a long-lived consumer that finishes the async provisioning
// workflow kicked off by an admin's earlier ApproveUser command. The
// audit log distinguishes "an admin approved" from "the system
// finalized provisioning"; reusing the user's own ID would conflate
// the two and surface a confusing "user X approved themselves" entry.
// The all-zeros UUID is the conventional choice for "no actor" in
// systems that still require a non-null actor column. Downstream
// consumers (audit-log projector, admin UI activity feed) special-case
// this value to render "system" instead of looking up a user row.
//
// Format note: the platform.users table has no row with this ID, and
// the audit-log projection MUST NOT attempt a foreign-key join on
// approved_by_user_id when this value appears. The Wave-2 audit-log
// PR will add the corresponding rendering rule.
const SystemActorUserID = "00000000-0000-0000-0000-000000000000"
