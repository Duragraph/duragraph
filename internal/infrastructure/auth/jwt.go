// Package auth contains the JWT issuance + verification primitives shared by
// the platform OAuth callback handler (which mints tokens) and the engine's
// HTTP middleware (which verifies them on every authenticated request).
//
// The claim shape implemented here is the one defined in
// duragraph-spec/auth/jwt.yml: HS256, claim names `user_id`, `tenant_id`,
// `role`, `email`, `iat`, `exp`, `iss`. Issuer is the constant
// `duragraph-platform`. tenant_id is optional (empty when the user's
// signup is awaiting operator approval — see auth/oauth.yml callback flow,
// case existing_user_pending). role is one of "user" or "admin".
//
// Earlier iterations of this file shipped a different claim set
// ({user_id, email, name, provider, exp, iat}) — that shape predates the
// multi-tenant platform model and is now replaced by this Claims type.
// `name` and `provider` are NOT carried in the canonical token; OAuth
// userinfo is the authoritative source for those, looked up via the
// platform User aggregate when needed.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// IssuerDuragraphPlatform is the value the platform auth layer signs into
// the `iss` claim and the only value the engine verifier accepts. Defending
// against accidental cross-product token reuse should JWT_SECRET ever leak
// into a sibling environment (per spec auth/jwt.yml § issuer).
const IssuerDuragraphPlatform = "duragraph-platform"

// Claims is the canonical session-token claim shape minted by the platform
// auth layer and consumed by the engine middleware.
//
// Field semantics (see auth/jwt.yml for the contract):
//   - UserID:   stable User aggregate ID. Always present.
//   - TenantID: Tenant aggregate ID. Empty/absent for pending users
//     (signup not yet approved). Engine route guards refuse /api/v1/* when
//     this is empty.
//   - Role:     authorization tier. "user" or "admin". Always present.
//   - Email:    OAuth-verified email, used for display + audit log entries.
//
// Standard claims (`iat`, `exp`, `iss`) live in the embedded
// jwt.RegisteredClaims; we MUST set Issuer to IssuerDuragraphPlatform when
// minting and verify it explicitly when parsing.
type Claims struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id,omitempty"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

// JWT verification errors. Callers (HTTP middleware, refresh endpoints) can
// distinguish via errors.Is to decide on 401 vs other handling.
var (
	// ErrTokenMalformed is returned when the token can't be parsed as a JWT.
	ErrTokenMalformed = errors.New("malformed jwt")
	// ErrTokenInvalidSignature is returned when the HMAC signature doesn't
	// verify against the configured secret.
	ErrTokenInvalidSignature = errors.New("invalid jwt signature")
	// ErrTokenExpired is returned when exp <= now.
	ErrTokenExpired = errors.New("jwt expired")
	// ErrTokenNotYetValid is returned when nbf is in the future. Not used
	// today (we never set nbf) but possible per RFC 7519.
	ErrTokenNotYetValid = errors.New("jwt not yet valid")
	// ErrTokenWrongIssuer is returned when the iss claim is missing or
	// doesn't equal IssuerDuragraphPlatform.
	ErrTokenWrongIssuer = errors.New("jwt issuer mismatch")
	// ErrTokenMissingClaim is returned when a required claim (user_id,
	// role, email) is absent.
	ErrTokenMissingClaim = errors.New("jwt missing required claim")
)

// IssueJWT mints a new HS256-signed session token. Exposed as a package-
// level helper rather than a method so non-OAuth callers (a future refresh
// endpoint, tests) can use it without instantiating an OAuthManager.
//
// secret    : shared symmetric key (engine + platform read the same one).
// userID    : User aggregate ID (stable across logins).
// email     : OAuth-verified email (display + audit).
// role      : "user" or "admin".
// tenantID  : Tenant aggregate ID. Pass "" for pending users — the
//
//	verifier accepts an empty tenant_id and route guards
//	(RequireTenant) reject /api/v1/* downstream.
//
// ttl       : token lifetime. Spec default is 24h; refresh threshold 6h.
//
// Returns the signed token string.
func IssueJWT(secret []byte, userID, email, role, tenantID string, ttl time.Duration) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("issue jwt: user_id required")
	}
	if role == "" {
		return "", fmt.Errorf("issue jwt: role required")
	}
	if email == "" {
		return "", fmt.Errorf("issue jwt: email required")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("issue jwt: ttl must be positive")
	}

	now := time.Now()
	claims := Claims{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IssuerDuragraphPlatform,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("issue jwt: sign: %w", err)
	}
	return signed, nil
}

// VerifyJWT parses and validates a session token. Returns the typed Claims
// on success, or one of the sentinel errors on failure.
//
// Validation steps:
//  1. Parse the JWT structure. ErrTokenMalformed on syntactic failure.
//  2. Verify the signing algorithm is HS256 (reject "none", RSA, etc.).
//  3. Verify the HMAC signature against secret. ErrTokenInvalidSignature.
//  4. Verify standard time claims (exp, nbf). ErrTokenExpired or
//     ErrTokenNotYetValid as appropriate.
//  5. Verify iss == IssuerDuragraphPlatform. ErrTokenWrongIssuer otherwise.
//     We do this AFTER signature verification — checking iss on a forged
//     token would be a meaningless step.
//  6. Verify required claims (user_id, role, email) are non-empty.
//
// The order matters: a malformed token must NEVER reach the issuer check
// (the issuer string in an unsigned token is attacker-controlled).
func VerifyJWT(secret []byte, tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrTokenMalformed
	}

	keyFunc := func(token *jwt.Token) (interface{}, error) {
		// Reject any non-HMAC signing method. A forged "alg: none" token
		// would otherwise pass without signature verification.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	}

	parsed, err := jwt.ParseWithClaims(tokenString, &Claims{}, keyFunc)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrTokenExpired
		case errors.Is(err, jwt.ErrTokenNotValidYet):
			return nil, ErrTokenNotYetValid
		case errors.Is(err, jwt.ErrTokenSignatureInvalid):
			return nil, ErrTokenInvalidSignature
		case errors.Is(err, jwt.ErrTokenMalformed):
			return nil, ErrTokenMalformed
		default:
			// Unknown parse error — treat as malformed rather than leaking
			// the raw library error string into responses.
			return nil, fmt.Errorf("%w: %v", ErrTokenMalformed, err)
		}
	}

	if !parsed.Valid {
		return nil, ErrTokenMalformed
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok {
		return nil, ErrTokenMalformed
	}

	// Explicit issuer check. golang-jwt v5's default validator does NOT
	// verify the iss claim unless you pass jwt.WithIssuer; we do it inline
	// here for a clearer single-pass error path.
	if claims.Issuer != IssuerDuragraphPlatform {
		return nil, ErrTokenWrongIssuer
	}

	// Required claims that the spec marks `required: true`.
	if claims.UserID == "" || claims.Role == "" || claims.Email == "" {
		return nil, ErrTokenMissingClaim
	}

	return claims, nil
}

// Verifier wraps the symmetric secret used to validate session tokens.
// HTTP middleware consumes a Verifier rather than the raw []byte so the
// secret stays encapsulated and tests can stub the type if needed.
type Verifier struct {
	secret []byte
}

// NewVerifier constructs a Verifier from the JWT_SECRET bytes. Empty
// secrets are rejected — running the engine with AUTH_ENABLED=true and a
// missing secret should be a hard configuration error caught at startup.
func NewVerifier(secret []byte) (*Verifier, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("new verifier: jwt secret is empty")
	}
	return &Verifier{secret: secret}, nil
}

// Verify parses + validates a token string. Pass-through to VerifyJWT;
// kept as a method so middleware can depend on the *Verifier type
// (clearer dependency injection point than a raw secret slice).
func (v *Verifier) Verify(tokenString string) (*Claims, error) {
	return VerifyJWT(v.secret, tokenString)
}
