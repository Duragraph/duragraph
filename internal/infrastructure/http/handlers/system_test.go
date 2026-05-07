package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestInfo_Capabilities exercises the capability fields on GET /info that
// power the embedded dashboard's admin gating.
func TestInfo_Capabilities(t *testing.T) {
	tests := []struct {
		name               string
		env                map[string]string
		wantMode           EngineMode
		wantPlatform       bool
		wantAuth           bool
		wantOAuthProviders []string
	}{
		{
			name:               "empty env defaults to serve mode with no providers",
			env:                map[string]string{},
			wantMode:           ModeServe,
			wantPlatform:       false,
			wantAuth:           false,
			wantOAuthProviders: []string{},
		},
		{
			name: "platform enabled flips mode to multitenant",
			env: map[string]string{
				"MIGRATOR_PLATFORM_ENABLED": "true",
			},
			wantMode:           ModeMultitenant,
			wantPlatform:       true,
			wantAuth:           false,
			wantOAuthProviders: []string{},
		},
		{
			name: "auth enabled toggle",
			env: map[string]string{
				"AUTH_ENABLED": "true",
			},
			wantMode:           ModeServe,
			wantPlatform:       false,
			wantAuth:           true,
			wantOAuthProviders: []string{},
		},
		{
			name: "google provider only",
			env: map[string]string{
				"OAUTH_GOOGLE_CLIENT_ID": "fake-google-id",
			},
			wantMode:           ModeServe,
			wantOAuthProviders: []string{"google"},
		},
		{
			name: "github provider only",
			env: map[string]string{
				"OAUTH_GITHUB_CLIENT_ID": "fake-github-id",
			},
			wantMode:           ModeServe,
			wantOAuthProviders: []string{"github"},
		},
		{
			name: "both providers, alphabetical order",
			env: map[string]string{
				"OAUTH_GOOGLE_CLIENT_ID": "g",
				"OAUTH_GITHUB_CLIENT_ID": "h",
			},
			wantMode:           ModeServe,
			wantOAuthProviders: []string{"github", "google"},
		},
		{
			name: "all flags together",
			env: map[string]string{
				"MIGRATOR_PLATFORM_ENABLED": "true",
				"AUTH_ENABLED":              "true",
				"OAUTH_GOOGLE_CLIENT_ID":    "g",
				"OAUTH_GITHUB_CLIENT_ID":    "h",
			},
			wantMode:           ModeMultitenant,
			wantPlatform:       true,
			wantAuth:           true,
			wantOAuthProviders: []string{"github", "google"},
		},
		{
			name: "platform flag with value other than true is treated as false",
			env: map[string]string{
				"MIGRATOR_PLATFORM_ENABLED": "1",
			},
			wantMode:           ModeServe,
			wantPlatform:       false,
			wantOAuthProviders: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv automatically restores prior values on test completion.
			// We also explicitly clear the four vars we care about so a value
			// leaked from the host env does not contaminate "absent" cases.
			for _, k := range []string{
				"MIGRATOR_PLATFORM_ENABLED",
				"AUTH_ENABLED",
				"OAUTH_GOOGLE_CLIENT_ID",
				"OAUTH_GITHUB_CLIENT_ID",
			} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/info", nil)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			h := NewSystemHandler("test-version")
			if err := h.Info(ctx); err != nil {
				t.Fatalf("Info handler returned error: %v", err)
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}

			var resp InfoResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
			}

			if resp.Mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", resp.Mode, tc.wantMode)
			}
			if resp.PlatformEnabled != tc.wantPlatform {
				t.Errorf("platform_enabled = %v, want %v", resp.PlatformEnabled, tc.wantPlatform)
			}
			if resp.AuthEnabled != tc.wantAuth {
				t.Errorf("auth_enabled = %v, want %v", resp.AuthEnabled, tc.wantAuth)
			}
			if !slicesEqual(resp.OAuthProviders, tc.wantOAuthProviders) {
				t.Errorf("oauth_providers = %v, want %v", resp.OAuthProviders, tc.wantOAuthProviders)
			}
		})
	}
}

// TestInfo_OAuthProvidersJSONShape verifies the JSON wire format renders an
// empty array (not null) when no providers are configured. This matters for
// the dashboard's TypeScript types, which expect `OAuthProvider[]`.
func TestInfo_OAuthProvidersJSONShape(t *testing.T) {
	for _, k := range []string{
		"MIGRATOR_PLATFORM_ENABLED",
		"AUTH_ENABLED",
		"OAUTH_GOOGLE_CLIENT_ID",
		"OAUTH_GITHUB_CLIENT_ID",
	} {
		t.Setenv(k, "")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	h := NewSystemHandler("test-version")
	if err := h.Info(ctx); err != nil {
		t.Fatalf("Info handler returned error: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"oauth_providers":[]`) {
		t.Errorf("expected oauth_providers to render as `[]`, got body=%s", body)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
