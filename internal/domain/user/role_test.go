package user

import "testing"

func TestRoleValid(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want bool
	}{
		{"user is valid", RoleUser, true},
		{"admin is valid", RoleAdmin, true},
		{"empty is invalid", Role(""), false},
		{"unknown is invalid", Role("operator"), false},
		{"capitalised is invalid (case-sensitive)", Role("User"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.want {
				t.Errorf("Role(%q).IsValid() = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestRoleString(t *testing.T) {
	if RoleUser.String() != "user" {
		t.Errorf("RoleUser.String() = %q, want %q", RoleUser.String(), "user")
	}
	if RoleAdmin.String() != "admin" {
		t.Errorf("RoleAdmin.String() = %q, want %q", RoleAdmin.String(), "admin")
	}
}
