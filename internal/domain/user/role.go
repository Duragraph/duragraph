package user

// Role represents the authorization tier of a platform user.
// Per auth/jwt.yml, the role claim drives access to admin surfaces:
//   - "user"  — dashboard + engine APIs (subject to tenant_id presence)
//   - "admin" — adds admin surfaces (/admin/*, /api/admin/*)
//
// The very first user to sign up is auto-elevated to admin (bootstrap path);
// subsequent users default to "user" and only an existing admin can promote
// them.
type Role string

const (
	// RoleUser is the default authorization tier for end users.
	RoleUser Role = "user"

	// RoleAdmin grants access to admin surfaces and operator-only commands.
	RoleAdmin Role = "admin"
)

// IsValid reports whether r is a recognized Role.
func (r Role) IsValid() bool {
	switch r {
	case RoleUser, RoleAdmin:
		return true
	}
	return false
}

// String returns the string representation of the Role.
func (r Role) String() string {
	return string(r)
}
