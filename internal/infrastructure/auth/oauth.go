package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

// Provider represents an OAuth provider
type Provider string

const (
	ProviderGoogle Provider = "google"
	ProviderGitHub Provider = "github"
)

// OAuthConfig holds OAuth configuration
type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	RedirectURL        string
	JWTSecret          string
	StateStore         StateStore // For storing OAuth state tokens
}

// StateStore interface for storing OAuth state
type StateStore interface {
	Set(ctx context.Context, state string, data interface{}, expiration time.Duration) error
	Get(ctx context.Context, state string) (interface{}, error)
	Delete(ctx context.Context, state string) error
}

// OAuthManager manages OAuth providers
type OAuthManager struct {
	configs    map[Provider]*oauth2.Config
	jwtSecret  []byte
	stateStore StateStore
}

// NewOAuthManager creates a new OAuth manager
func NewOAuthManager(config OAuthConfig) *OAuthManager {
	manager := &OAuthManager{
		configs:    make(map[Provider]*oauth2.Config),
		jwtSecret:  []byte(config.JWTSecret),
		stateStore: config.StateStore,
	}

	// Setup Google OAuth
	if config.GoogleClientID != "" {
		manager.configs[ProviderGoogle] = &oauth2.Config{
			ClientID:     config.GoogleClientID,
			ClientSecret: config.GoogleClientSecret,
			RedirectURL:  config.RedirectURL + "/google/callback",
			Scopes: []string{
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/userinfo.profile",
			},
			Endpoint: google.Endpoint,
		}
	}

	// Setup GitHub OAuth
	if config.GitHubClientID != "" {
		manager.configs[ProviderGitHub] = &oauth2.Config{
			ClientID:     config.GitHubClientID,
			ClientSecret: config.GitHubClientSecret,
			RedirectURL:  config.RedirectURL + "/github/callback",
			Scopes:       []string{"user:email", "read:user"},
			Endpoint:     github.Endpoint,
		}
	}

	return manager
}

// LoginHandler returns OAuth login handler
func (m *OAuthManager) LoginHandler(provider Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		config, exists := m.configs[provider]
		if !exists {
			return echo.NewHTTPError(http.StatusBadRequest, "Provider not configured")
		}

		// Generate state token
		state, err := generateStateToken()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate state")
		}

		// Store state with expiration
		if err := m.stateStore.Set(c.Request().Context(), state, provider, 10*time.Minute); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to store state")
		}

		// Redirect to OAuth provider
		url := config.AuthCodeURL(state)
		return c.Redirect(http.StatusTemporaryRedirect, url)
	}
}

// CallbackHandler returns OAuth callback handler
func (m *OAuthManager) CallbackHandler(provider Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		config, exists := m.configs[provider]
		if !exists {
			return echo.NewHTTPError(http.StatusBadRequest, "Provider not configured")
		}

		// Verify state
		state := c.QueryParam("state")
		if state == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "Missing state")
		}

		storedProvider, err := m.stateStore.Get(c.Request().Context(), state)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid state")
		}

		if storedProvider != provider {
			return echo.NewHTTPError(http.StatusBadRequest, "State mismatch")
		}

		// Delete used state
		m.stateStore.Delete(c.Request().Context(), state)

		// Exchange code for token
		code := c.QueryParam("code")
		if code == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "Missing code")
		}

		token, err := config.Exchange(c.Request().Context(), code)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to exchange token")
		}

		// Get user info
		userInfo, err := m.getUserInfo(c.Request().Context(), provider, token)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user info")
		}

		// Generate JWT
		jwtToken, err := m.generateJWT(userInfo)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate JWT")
		}

		// Return JWT token
		return c.JSON(http.StatusOK, map[string]interface{}{
			"token":    jwtToken,
			"user":     userInfo,
			"provider": provider,
		})
	}
}

// UserInfo represents user information from OAuth
type UserInfo struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Picture  string `json:"picture,omitempty"`
	Provider string `json:"provider"`
}

// getUserInfo fetches user info from OAuth provider
func (m *OAuthManager) getUserInfo(ctx context.Context, provider Provider, token *oauth2.Token) (*UserInfo, error) {
	config := m.configs[provider]
	client := config.Client(ctx, token)

	var userInfo UserInfo
	userInfo.Provider = string(provider)

	switch provider {
	case ProviderGoogle:
		resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var googleUser struct {
			ID      string `json:"id"`
			Email   string `json:"email"`
			Name    string `json:"name"`
			Picture string `json:"picture"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
			return nil, err
		}

		userInfo.ID = googleUser.ID
		userInfo.Email = googleUser.Email
		userInfo.Name = googleUser.Name
		userInfo.Picture = googleUser.Picture

	case ProviderGitHub:
		resp, err := client.Get("https://api.github.com/user")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var githubUser struct {
			ID        int    `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&githubUser); err != nil {
			return nil, err
		}

		userInfo.ID = fmt.Sprintf("%d", githubUser.ID)
		userInfo.Email = githubUser.Email
		userInfo.Name = githubUser.Name
		if userInfo.Name == "" {
			userInfo.Name = githubUser.Login
		}
		userInfo.Picture = githubUser.AvatarURL

		// GitHub might not return email in main response, fetch separately
		if userInfo.Email == "" {
			emailResp, err := client.Get("https://api.github.com/user/emails")
			if err == nil {
				defer emailResp.Body.Close()

				var emails []struct {
					Email    string `json:"email"`
					Primary  bool   `json:"primary"`
					Verified bool   `json:"verified"`
				}

				if err := json.NewDecoder(emailResp.Body).Decode(&emails); err == nil {
					for _, email := range emails {
						if email.Primary && email.Verified {
							userInfo.Email = email.Email
							break
						}
					}
				}
			}
		}
	}

	return &userInfo, nil
}

// generateJWT creates a JWT token for the user
func (m *OAuthManager) generateJWT(userInfo *UserInfo) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userInfo.ID,
		"email":    userInfo.Email,
		"name":     userInfo.Name,
		"provider": userInfo.Provider,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.jwtSecret)
}

// generateStateToken generates a random state token
func generateStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
