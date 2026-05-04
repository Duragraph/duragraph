package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"
)

// ProviderConfig bundles the provider-side configuration sourced from
// environment variables. Only providers with a non-empty client_id are
// registered with goth — passing an empty client_id silently skips that
// provider (matches the spec's "configurable_via_env" semantics, where a
// deployment can elect to enable Google only, GitHub only, or both).
type ProviderConfig struct {
	BaseURL string // e.g. https://platform.duragraph.ai

	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string

	// SessionSecret keys the gorilla/sessions cookie store goth uses to
	// hold the OAuth state token between /login and /callback. Spec
	// auth/oauth.yml § state_csrf identifies this cookie as
	// `_gothic_session`.
	//
	// With a single key, sessions.NewCookieStore uses HMAC-SHA256 for
	// integrity-only signing of the state cookie (encryption is opt-in
	// by passing a second blockKey; we don't — the CSRF state token is
	// short-lived and integrity is sufficient). Recommend at least 32
	// bytes of entropy; longer is fine but provides no extra benefit
	// past the HMAC-SHA256 block boundary (~64 bytes). The constructor
	// rejects anything shorter than 32 bytes so misconfigurations fail
	// at startup rather than silently weakening the state-cookie defence.
	SessionSecret string

	// CookieSecure controls the Secure attribute on the gothic state
	// cookie. True in production (HTTPS), false in dev.
	CookieSecure bool
}

// ConfigureProviders registers the configured providers with goth and
// initialises gothic.Store with a secure cookie store keyed off
// SessionSecret. Safe to call multiple times — the call is idempotent
// per goth's documented "duplicate providers are overwritten" semantics.
//
// Returns an error if BaseURL or SessionSecret is empty, or if no
// providers are configured (zero providers means no working OAuth flow,
// which is almost certainly a misconfiguration rather than an
// intentional choice).
func ConfigureProviders(cfg ProviderConfig) error {
	if cfg.BaseURL == "" {
		return fmt.Errorf("configure providers: BaseURL is required")
	}
	// SessionSecret keys the HMAC-SHA256 signature on the gothic state
	// cookie. 32 bytes matches HMAC-SHA256's natural key size and is the
	// shortest length that gives full-strength signatures; reject anything
	// shorter at startup.
	if len(cfg.SessionSecret) < 32 {
		return fmt.Errorf("configure providers: SessionSecret must be at least 32 bytes (got %d)", len(cfg.SessionSecret))
	}

	// Configure the gothic session store BEFORE goth.UseProviders. The
	// default gothic store is `sessions.NewCookieStore([]byte(""))` —
	// an empty key yields a forgeable state cookie, defeating the whole
	// CSRF defence. Always replace it.
	store := sessions.NewCookieStore([]byte(cfg.SessionSecret))
	store.Options.Path = "/"
	store.Options.HttpOnly = true
	store.Options.Secure = cfg.CookieSecure
	store.Options.SameSite = http.SameSiteLaxMode
	gothic.Store = store

	base := strings.TrimRight(cfg.BaseURL, "/")
	providers := make([]goth.Provider, 0, 2)

	if cfg.GoogleClientID != "" {
		providers = append(providers, google.New(
			cfg.GoogleClientID,
			cfg.GoogleClientSecret,
			base+"/api/auth/google/callback",
			"openid", "email", "profile",
		))
	}
	if cfg.GitHubClientID != "" {
		providers = append(providers, github.New(
			cfg.GitHubClientID,
			cfg.GitHubClientSecret,
			base+"/api/auth/github/callback",
			"user:email", "read:user",
		))
	}

	if len(providers) == 0 {
		return fmt.Errorf("configure providers: at least one provider must be configured (set OAUTH_GOOGLE_CLIENT_ID and/or OAUTH_GITHUB_CLIENT_ID)")
	}

	goth.UseProviders(providers...)
	return nil
}
