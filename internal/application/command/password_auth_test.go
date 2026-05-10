package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/mocks"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// newTestRegisterHandler returns a handler at bcrypt.MinCost so the
// suite stays fast (cost 4 ~ms vs cost 12 ~250ms × N tests).
func newTestRegisterHandler(repo user.Repository) *RegisterUserWithPasswordHandler {
	return NewRegisterUserWithPasswordHandlerWithCost(repo, bcrypt.MinCost)
}

func TestRegister_FirstUserBecomesAdmin(t *testing.T) {
	repo := mocks.NewUserRepository()
	h := newTestRegisterHandler(repo)

	id, err := h.Handle(context.Background(), RegisterUserWithPassword{
		Email:       "first@example.com",
		Password:    "swordfish",
		DisplayName: "First",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty user ID")
	}

	saved, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("user not saved: %v", err)
	}
	if saved.Role() != user.RoleAdmin {
		t.Errorf("first user role: got %v want admin", saved.Role())
	}
	if saved.Status() != user.StatusApproved {
		t.Errorf("first user status: got %v want approved", saved.Status())
	}
	if !saved.HasPassword() {
		t.Error("first user should have a password set")
	}
}

func TestRegister_SecondUserPending(t *testing.T) {
	repo := mocks.NewUserRepository()
	h := newTestRegisterHandler(repo)

	if _, err := h.Handle(context.Background(), RegisterUserWithPassword{
		Email: "first@example.com", Password: "swordfish", DisplayName: "First",
	}); err != nil {
		t.Fatalf("first register: %v", err)
	}

	id, err := h.Handle(context.Background(), RegisterUserWithPassword{
		Email: "second@example.com", Password: "swordfish", DisplayName: "Second",
	})
	if err != nil {
		t.Fatalf("second register: %v", err)
	}

	saved, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Role() != user.RoleUser {
		t.Errorf("second user role: got %v want user", saved.Role())
	}
	if saved.Status() != user.StatusPending {
		t.Errorf("second user status: got %v want pending", saved.Status())
	}
}

func TestRegister_RejectsDuplicateEmail(t *testing.T) {
	repo := mocks.NewUserRepository()
	h := newTestRegisterHandler(repo)

	if _, err := h.Handle(context.Background(), RegisterUserWithPassword{
		Email: "dup@example.com", Password: "swordfish", DisplayName: "Dup",
	}); err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, err := h.Handle(context.Background(), RegisterUserWithPassword{
		Email: "dup@example.com", Password: "swordfish", DisplayName: "Dup2",
	})
	if !errors.Is(err, pkgerrors.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got: %v", err)
	}
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	repo := mocks.NewUserRepository()
	h := newTestRegisterHandler(repo)

	_, err := h.Handle(context.Background(), RegisterUserWithPassword{
		Email: "x@example.com", Password: "short", DisplayName: "X",
	})
	if err == nil {
		t.Fatal("expected error for short password")
	}
	if !errors.Is(err, pkgerrors.ErrInvalidInput) {
		t.Errorf("error should be ErrInvalidInput: %v", err)
	}
}

func TestRegister_RejectsLongPassword(t *testing.T) {
	repo := mocks.NewUserRepository()
	h := newTestRegisterHandler(repo)

	_, err := h.Handle(context.Background(), RegisterUserWithPassword{
		Email:       "x@example.com",
		Password:    strings.Repeat("a", MaxPasswordLength+1),
		DisplayName: "X",
	})
	if err == nil {
		t.Fatal("expected error for >72-byte password")
	}
}

func TestRegister_RejectsEmptyEmail(t *testing.T) {
	repo := mocks.NewUserRepository()
	h := newTestRegisterHandler(repo)

	_, err := h.Handle(context.Background(), RegisterUserWithPassword{
		Email: "  ", Password: "swordfish", DisplayName: "X",
	})
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

// --- LoginWithPassword tests ---

func TestLogin_Success(t *testing.T) {
	repo := mocks.NewUserRepository()
	reg := newTestRegisterHandler(repo)
	id, err := reg.Handle(context.Background(), RegisterUserWithPassword{
		Email: "first@example.com", Password: "swordfish", DisplayName: "First",
	})
	if err != nil {
		t.Fatal(err)
	}

	login := NewLoginWithPasswordHandler(repo)
	u, err := login.Handle(context.Background(), LoginWithPassword{
		Email: "first@example.com", Password: "swordfish",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if u.ID() != id {
		t.Errorf("login returned wrong user: got %s want %s", u.ID(), id)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := mocks.NewUserRepository()
	reg := newTestRegisterHandler(repo)
	if _, err := reg.Handle(context.Background(), RegisterUserWithPassword{
		Email: "first@example.com", Password: "swordfish", DisplayName: "First",
	}); err != nil {
		t.Fatal(err)
	}

	login := NewLoginWithPasswordHandler(repo)
	_, err := login.Handle(context.Background(), LoginWithPassword{
		Email: "first@example.com", Password: "wrongguess",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	repo := mocks.NewUserRepository()
	login := NewLoginWithPasswordHandler(repo)

	_, err := login.Handle(context.Background(), LoginWithPassword{
		Email: "ghost@example.com", Password: "anything",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown email should return ErrInvalidCredentials, got: %v", err)
	}
}

func TestLogin_PendingUserBlocked(t *testing.T) {
	// Second user → pending; pending users must NOT be able to log in.
	repo := mocks.NewUserRepository()
	reg := newTestRegisterHandler(repo)
	if _, err := reg.Handle(context.Background(), RegisterUserWithPassword{
		Email: "first@example.com", Password: "swordfish", DisplayName: "First",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Handle(context.Background(), RegisterUserWithPassword{
		Email: "pending@example.com", Password: "swordfish", DisplayName: "Pending",
	}); err != nil {
		t.Fatal(err)
	}

	login := NewLoginWithPasswordHandler(repo)
	_, err := login.Handle(context.Background(), LoginWithPassword{
		Email: "pending@example.com", Password: "swordfish",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("pending user login should return ErrInvalidCredentials, got: %v", err)
	}
}

func TestLogin_OAuthOnlyUserBlocked(t *testing.T) {
	// User registered via OAuth has no password_hash. Login attempt
	// must return ErrInvalidCredentials, not crash on bcrypt against
	// empty hash.
	repo := mocks.NewUserRepository()
	u, err := user.RegisterUser("oauth@example.com", "google", "google-id-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(context.Background(), u); err != nil {
		t.Fatal(err)
	}

	login := NewLoginWithPasswordHandler(repo)
	_, err = login.Handle(context.Background(), LoginWithPassword{
		Email: "oauth@example.com", Password: "anything",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("OAuth-only user login should return ErrInvalidCredentials, got: %v", err)
	}
}

func TestLogin_CaseInsensitiveEmail(t *testing.T) {
	repo := mocks.NewUserRepository()
	reg := newTestRegisterHandler(repo)
	if _, err := reg.Handle(context.Background(), RegisterUserWithPassword{
		Email: "Mixed@Example.COM", Password: "swordfish", DisplayName: "Case",
	}); err != nil {
		t.Fatal(err)
	}

	login := NewLoginWithPasswordHandler(repo)
	if _, err := login.Handle(context.Background(), LoginWithPassword{
		Email: "mixed@example.com", Password: "swordfish",
	}); err != nil {
		t.Errorf("login should be case-insensitive: %v", err)
	}
}
