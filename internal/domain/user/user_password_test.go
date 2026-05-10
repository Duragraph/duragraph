package user

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// hashForTest produces a low-cost bcrypt hash for tests. Real callers use
// cost 12 (~250ms); tests use the lib's MinCost (4, ~ms-range) to keep
// the suite fast.
func hashForTest(t *testing.T, plaintext string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash %q: %v", plaintext, err)
	}
	return string(h)
}

func TestRegisterWithPassword_Normal(t *testing.T) {
	hash := hashForTest(t, "swordfish")

	u, err := RegisterWithPassword("alice@example.com", hash, "Alice", false)
	if err != nil {
		t.Fatalf("RegisterWithPassword: %v", err)
	}

	if u.Email() != "alice@example.com" {
		t.Errorf("email: got %q", u.Email())
	}
	if u.OAuthProvider() != "" {
		t.Errorf("oauth_provider should be empty for password user, got %q", u.OAuthProvider())
	}
	if !u.HasPassword() {
		t.Error("HasPassword() should be true")
	}
	if u.AuthMethod() != "password" {
		t.Errorf("auth_method: got %q want password", u.AuthMethod())
	}
	if u.Role() != RoleUser {
		t.Errorf("role: got %v want user", u.Role())
	}
	if u.Status() != StatusPending {
		t.Errorf("status: got %v want pending", u.Status())
	}

	events := u.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event (UserRegisteredWithPassword), got %d", len(events))
	}
	if events[0].EventType() != EventTypeUserRegisteredWithPassword {
		t.Errorf("event type: got %s", events[0].EventType())
	}
}

func TestRegisterWithPassword_Bootstrap(t *testing.T) {
	hash := hashForTest(t, "swordfish")

	u, err := RegisterWithPassword("first@example.com", hash, "First", true)
	if err != nil {
		t.Fatalf("RegisterWithPassword: %v", err)
	}

	if u.Role() != RoleAdmin {
		t.Errorf("bootstrap user role: got %v want admin", u.Role())
	}
	if u.Status() != StatusApproved {
		t.Errorf("bootstrap user status: got %v want approved", u.Status())
	}

	events := u.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events (registered+promoted+approved), got %d", len(events))
	}
	wantTypes := []string{
		EventTypeUserRegisteredWithPassword,
		EventTypeUserPromotedToAdmin,
		EventTypeUserApproved,
	}
	for i, want := range wantTypes {
		if got := events[i].EventType(); got != want {
			t.Errorf("event[%d]: got %s want %s", i, got, want)
		}
	}
}

func TestRegisterWithPassword_RejectsEmptyInputs(t *testing.T) {
	hash := hashForTest(t, "swordfish")

	cases := []struct {
		name              string
		email, hash, disp string
	}{
		{"empty email", "", hash, "x"},
		{"empty hash", "a@b.com", "", "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RegisterWithPassword(tc.email, tc.hash, tc.disp, false)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestVerifyPassword_Match(t *testing.T) {
	hash := hashForTest(t, "swordfish")

	u, err := RegisterWithPassword("a@b.com", hash, "x", false)
	if err != nil {
		t.Fatal(err)
	}
	if !u.VerifyPassword("swordfish") {
		t.Error("VerifyPassword should return true for correct password")
	}
	if u.VerifyPassword("wrongguess") {
		t.Error("VerifyPassword should return false for wrong password")
	}
}

func TestVerifyPassword_OAuthOnlyUserAlwaysFalse(t *testing.T) {
	// User registered via OAuth has no password_hash. VerifyPassword
	// must return false WITHOUT calling bcrypt (would otherwise be a
	// timing-attack vector — bcrypt.Compare on empty hash takes the
	// same ~250ms as a real comparison and reveals account state).
	u, err := RegisterUser("oauth@example.com", "google", "google-id-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if u.HasPassword() {
		t.Error("OAuth-only user should not have a password")
	}
	if u.VerifyPassword("anything") {
		t.Error("VerifyPassword on OAuth-only user must return false")
	}
}

func TestSetPassword_Updates(t *testing.T) {
	hash := hashForTest(t, "old-pw")

	u, err := RegisterWithPassword("a@b.com", hash, "x", false)
	if err != nil {
		t.Fatal(err)
	}
	u.ClearEvents()

	newHash := hashForTest(t, "new-pw")
	if err := u.SetPassword(newHash, u.ID()); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	if !u.VerifyPassword("new-pw") {
		t.Error("after SetPassword, new password should verify")
	}
	if u.VerifyPassword("old-pw") {
		t.Error("after SetPassword, old password should NOT verify")
	}

	events := u.Events()
	if len(events) != 1 || events[0].EventType() != EventTypeUserPasswordChanged {
		t.Errorf("expected one UserPasswordChanged, got %d events", len(events))
	}
}

func TestSetPassword_RejectsSuspended(t *testing.T) {
	hash := hashForTest(t, "old-pw")
	u, _ := RegisterWithPassword("a@b.com", hash, "x", false)

	// Force into approved → suspended.
	if err := u.Approve("admin-id"); err != nil {
		t.Fatal(err)
	}
	if err := u.Suspend("admin-id", "test"); err != nil {
		t.Fatal(err)
	}

	newHash := hashForTest(t, "new-pw")
	err := u.SetPassword(newHash, "admin-id")
	if err == nil {
		t.Error("SetPassword should reject suspended user")
	}
	if !strings.Contains(err.Error(), "suspended") {
		t.Errorf("error should mention suspended status: %v", err)
	}
}

func TestSetPassword_PendingUserAccepted(t *testing.T) {
	// Pending users CAN have their password reset (e.g., admin-driven
	// reset before approval). Only suspended is the hard block.
	hash := hashForTest(t, "old-pw")
	u, _ := RegisterWithPassword("a@b.com", hash, "x", false)

	newHash := hashForTest(t, "reset-pw")
	if err := u.SetPassword(newHash, "admin-id"); err != nil {
		t.Errorf("SetPassword on pending user should succeed: %v", err)
	}
	if !u.VerifyPassword("reset-pw") {
		t.Error("post-reset password should verify")
	}
}

func TestReconstructFromData_PasswordFields(t *testing.T) {
	hash := hashForTest(t, "swordfish")
	u := ReconstructFromData(UserData{
		ID:           "u1",
		Email:        "a@b.com",
		PasswordHash: hash,
		AuthMethod:   "password",
		Role:         "user",
		Status:       "approved",
	})
	if !u.HasPassword() {
		t.Error("HasPassword should be true after reconstruct with hash")
	}
	if u.AuthMethod() != "password" {
		t.Errorf("auth_method: got %q", u.AuthMethod())
	}
	if !u.VerifyPassword("swordfish") {
		t.Error("reconstructed user's hash should verify against original plaintext")
	}
}

func TestReconstructFromData_DefaultsAuthMethodToOAuth(t *testing.T) {
	// Back-compat: pre-005-migration rows have no auth_method column;
	// ReconstructFromData should default it to "oauth".
	u := ReconstructFromData(UserData{
		ID:            "u1",
		Email:         "a@b.com",
		OAuthProvider: "google",
		OAuthID:       "google-1",
		// AuthMethod intentionally empty
		Role:   "user",
		Status: "approved",
	})
	if u.AuthMethod() != "oauth" {
		t.Errorf("auth_method default: got %q want oauth", u.AuthMethod())
	}
}
