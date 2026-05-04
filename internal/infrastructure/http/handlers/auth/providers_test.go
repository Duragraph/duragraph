package auth

import (
	"strings"
	"testing"

	"github.com/markbates/goth"
)

// ConfigureProviders rejects an empty BaseURL outright.
func TestConfigureProviders_EmptyBaseURL(t *testing.T) {
	withClearedProviders(t)
	err := ConfigureProviders(ProviderConfig{
		BaseURL:        "",
		SessionSecret:  strings.Repeat("a", 32),
		GoogleClientID: "x",
	})
	if err == nil {
		t.Fatal("expected error when BaseURL is empty")
	}
}

// SessionSecret < 32 bytes is rejected so misconfigurations fail at startup
// rather than silently weakening the gothic state-cookie HMAC.
func TestConfigureProviders_ShortSessionSecret(t *testing.T) {
	withClearedProviders(t)
	err := ConfigureProviders(ProviderConfig{
		BaseURL:        "https://platform.example.com",
		SessionSecret:  "too-short", // 9 bytes
		GoogleClientID: "x",
	})
	if err == nil {
		t.Fatal("expected error for short SessionSecret")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("error should mention 32-byte minimum, got: %v", err)
	}
}

// Empty SessionSecret hits the same check (a degenerate "less than 32 bytes"
// case). Worth a dedicated test so the message stays helpful for the most
// common misconfiguration: the env var unset entirely.
func TestConfigureProviders_EmptySessionSecret(t *testing.T) {
	withClearedProviders(t)
	err := ConfigureProviders(ProviderConfig{
		BaseURL:        "https://platform.example.com",
		SessionSecret:  "",
		GoogleClientID: "x",
	})
	if err == nil {
		t.Fatal("expected error for empty SessionSecret")
	}
}

// 32-byte exact minimum + at least one provider configured → success path.
// Verifies the validation isn't off-by-one (>= 32 not > 32) and that the
// happy path still wires goth.
func TestConfigureProviders_MinimumSecretLengthAccepted(t *testing.T) {
	withClearedProviders(t)
	err := ConfigureProviders(ProviderConfig{
		BaseURL:        "https://platform.example.com",
		SessionSecret:  strings.Repeat("a", 32),
		GoogleClientID: "google-id",
		GitHubClientID: "github-id",
	})
	if err != nil {
		t.Fatalf("expected success at 32-byte secret, got: %v", err)
	}
	if _, err := goth.GetProvider("google"); err != nil {
		t.Errorf("google provider should be registered after success: %v", err)
	}
}

// Zero providers configured (both client IDs empty) → error. Ensures the
// existing "at least one provider" guard still fires after our SessionSecret
// length validation lands earlier in the function.
func TestConfigureProviders_NoProviders(t *testing.T) {
	withClearedProviders(t)
	err := ConfigureProviders(ProviderConfig{
		BaseURL:       "https://platform.example.com",
		SessionSecret: strings.Repeat("a", 32),
	})
	if err == nil {
		t.Fatal("expected error when no providers configured")
	}
}
