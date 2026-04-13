package auth

import (
	"context"
	"testing"
	"time"
)

func TestNewOAuthManager_Google(t *testing.T) {
	m := NewOAuthManager(OAuthConfig{
		GoogleClientID:     "google-id",
		GoogleClientSecret: "google-secret",
		RedirectURL:        "http://localhost:8080/auth",
		JWTSecret:          "secret",
	})

	if _, ok := m.configs[ProviderGoogle]; !ok {
		t.Error("Google provider should be configured")
	}
	if _, ok := m.configs[ProviderGitHub]; ok {
		t.Error("GitHub provider should not be configured")
	}
}

func TestNewOAuthManager_GitHub(t *testing.T) {
	m := NewOAuthManager(OAuthConfig{
		GitHubClientID:     "github-id",
		GitHubClientSecret: "github-secret",
		RedirectURL:        "http://localhost:8080/auth",
		JWTSecret:          "secret",
	})

	if _, ok := m.configs[ProviderGitHub]; !ok {
		t.Error("GitHub provider should be configured")
	}
	if _, ok := m.configs[ProviderGoogle]; ok {
		t.Error("Google provider should not be configured")
	}
}

func TestNewOAuthManager_Both(t *testing.T) {
	m := NewOAuthManager(OAuthConfig{
		GoogleClientID:     "g-id",
		GoogleClientSecret: "g-secret",
		GitHubClientID:     "gh-id",
		GitHubClientSecret: "gh-secret",
		RedirectURL:        "http://localhost:8080/auth",
		JWTSecret:          "secret",
	})

	if len(m.configs) != 2 {
		t.Errorf("expected 2 providers, got %d", len(m.configs))
	}
}

func TestNewOAuthManager_NoProviders(t *testing.T) {
	m := NewOAuthManager(OAuthConfig{
		JWTSecret: "secret",
	})

	if len(m.configs) != 0 {
		t.Errorf("expected 0 providers, got %d", len(m.configs))
	}
}

func TestNewOAuthManager_RedirectURLs(t *testing.T) {
	m := NewOAuthManager(OAuthConfig{
		GoogleClientID:     "g-id",
		GoogleClientSecret: "g-secret",
		GitHubClientID:     "gh-id",
		GitHubClientSecret: "gh-secret",
		RedirectURL:        "http://localhost:8080/auth",
		JWTSecret:          "secret",
	})

	googleConfig := m.configs[ProviderGoogle]
	if googleConfig.RedirectURL != "http://localhost:8080/auth/google/callback" {
		t.Errorf("Google redirect: got %q", googleConfig.RedirectURL)
	}

	githubConfig := m.configs[ProviderGitHub]
	if githubConfig.RedirectURL != "http://localhost:8080/auth/github/callback" {
		t.Errorf("GitHub redirect: got %q", githubConfig.RedirectURL)
	}
}

func TestGenerateJWT(t *testing.T) {
	m := NewOAuthManager(OAuthConfig{
		JWTSecret: "test-secret-key",
	})

	userInfo := &UserInfo{
		ID:       "user-123",
		Email:    "test@example.com",
		Name:     "Test User",
		Provider: "google",
	}

	token, err := m.generateJWT(userInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("token should not be empty")
	}
}

func TestGenerateStateToken(t *testing.T) {
	token1, err := generateStateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(token1) == 0 {
		t.Error("token should not be empty")
	}

	token2, err := generateStateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token1 == token2 {
		t.Error("tokens should be unique")
	}
}

func TestProviderConstants(t *testing.T) {
	if ProviderGoogle != "google" {
		t.Errorf("expected 'google', got %q", ProviderGoogle)
	}
	if ProviderGitHub != "github" {
		t.Errorf("expected 'github', got %q", ProviderGitHub)
	}
}

func TestUserInfo_Fields(t *testing.T) {
	u := UserInfo{
		ID:       "123",
		Email:    "test@test.com",
		Name:     "Test",
		Picture:  "http://pic.url",
		Provider: "google",
	}

	if u.ID != "123" || u.Email != "test@test.com" || u.Provider != "google" {
		t.Errorf("unexpected: %+v", u)
	}
}

func TestOAuthConfig_StateStore(t *testing.T) {
	config := OAuthConfig{
		JWTSecret:  "secret",
		StateStore: &mockStateStore{},
	}

	m := NewOAuthManager(config)
	if m.stateStore == nil {
		t.Error("state store should be set")
	}
}

type mockStateStore struct{}

func (s *mockStateStore) Set(_ context.Context, _ string, _ interface{}, _ time.Duration) error {
	return nil
}
func (s *mockStateStore) Get(_ context.Context, _ string) (interface{}, error) {
	return nil, nil
}
func (s *mockStateStore) Delete(_ context.Context, _ string) error {
	return nil
}
