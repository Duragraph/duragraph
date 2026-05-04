package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TestIssueJWT_RoundTrip mints a token with the canonical shape and
// confirms VerifyJWT returns identical claim values. This is the happy-
// path contract enforcement: anything we put in MUST come back out.
func TestIssueJWT_RoundTrip(t *testing.T) {
	secret := []byte("test-secret-key")

	token, err := IssueJWT(
		secret,
		"user-123",
		"alice@example.com",
		"admin",
		"tenant-abc",
		time.Hour,
	)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}
	if token == "" {
		t.Fatal("token must not be empty")
	}

	claims, err := VerifyJWT(secret, token)
	if err != nil {
		t.Fatalf("VerifyJWT: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("UserID: got %q want user-123", claims.UserID)
	}
	if claims.TenantID != "tenant-abc" {
		t.Errorf("TenantID: got %q want tenant-abc", claims.TenantID)
	}
	if claims.Role != "admin" {
		t.Errorf("Role: got %q want admin", claims.Role)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("Email: got %q want alice@example.com", claims.Email)
	}
	if claims.Issuer != IssuerDuragraphPlatform {
		t.Errorf("Issuer: got %q want %q", claims.Issuer, IssuerDuragraphPlatform)
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatal("iat/exp claims must be set")
	}
	if !claims.ExpiresAt.After(claims.IssuedAt.Time) {
		t.Errorf("exp must be after iat: iat=%v exp=%v", claims.IssuedAt, claims.ExpiresAt)
	}
}

// TestIssueJWT_PendingUser confirms an empty tenant_id is round-tripped
// faithfully — pending users (signup not yet approved) get a token with
// no tenant. The verifier MUST accept this; route guards
// (RequireTenant) reject downstream.
func TestIssueJWT_PendingUser(t *testing.T) {
	secret := []byte("test-secret-key")

	token, err := IssueJWT(secret, "user-pending", "bob@example.com", "user", "", time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}

	claims, err := VerifyJWT(secret, token)
	if err != nil {
		t.Fatalf("VerifyJWT: %v", err)
	}
	if claims.TenantID != "" {
		t.Errorf("TenantID for pending user: got %q want empty", claims.TenantID)
	}
	if claims.Role != "user" {
		t.Errorf("Role: got %q want user", claims.Role)
	}
}

// TestIssueJWT_RejectsMissingFields is a guard against silent typos at
// callsites — IssueJWT must hard-fail rather than mint a token that
// would later be rejected by the verifier.
func TestIssueJWT_RejectsMissingFields(t *testing.T) {
	secret := []byte("secret")

	cases := []struct {
		name     string
		userID   string
		email    string
		role     string
		tenantID string
		ttl      time.Duration
	}{
		{"missing user_id", "", "a@b", "user", "t", time.Hour},
		{"missing email", "u", "", "user", "t", time.Hour},
		{"missing role", "u", "a@b", "", "t", time.Hour},
		{"zero ttl", "u", "a@b", "user", "t", 0},
		{"negative ttl", "u", "a@b", "user", "t", -time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := IssueJWT(secret, tc.userID, tc.email, tc.role, tc.tenantID, tc.ttl)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

// TestVerifyJWT_Expired ensures expired tokens are explicitly rejected.
// The default golang-jwt v5 validator handles this; we just confirm the
// sentinel error mapping.
func TestVerifyJWT_Expired(t *testing.T) {
	secret := []byte("secret")

	// Mint with negative ttl — exp will be in the past.
	claims := Claims{
		UserID: "u",
		Role:   "user",
		Email:  "a@b",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IssuerDuragraphPlatform,
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = VerifyJWT(secret, signed)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

// TestVerifyJWT_WrongIssuer is the cross-product token-leak guard. Tokens
// signed with the same secret but issued by a different system MUST be
// rejected — that's exactly why iss is in the spec.
func TestVerifyJWT_WrongIssuer(t *testing.T) {
	secret := []byte("secret")

	claims := Claims{
		UserID: "u",
		Role:   "user",
		Email:  "a@b",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "some-other-product",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString(secret)

	_, err := VerifyJWT(secret, signed)
	if !errors.Is(err, ErrTokenWrongIssuer) {
		t.Errorf("expected ErrTokenWrongIssuer, got %v", err)
	}
}

// TestVerifyJWT_BadSignature confirms that a token signed with a
// different secret is rejected at signature verification — i.e. before
// the iss check runs (a forged token's iss is attacker-controlled).
func TestVerifyJWT_BadSignature(t *testing.T) {
	good := []byte("correct-secret")
	bad := []byte("wrong-secret")

	signed, err := IssueJWT(good, "u", "a@b", "user", "", time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}

	_, err = VerifyJWT(bad, signed)
	if !errors.Is(err, ErrTokenInvalidSignature) {
		t.Errorf("expected ErrTokenInvalidSignature, got %v", err)
	}
}

// TestVerifyJWT_Malformed exercises the syntactic-failure path —
// garbage in, ErrTokenMalformed out.
func TestVerifyJWT_Malformed(t *testing.T) {
	secret := []byte("secret")

	cases := []string{
		"",
		"not-a-jwt",
		"a.b",                    // wrong number of segments
		"header.payload.garbage", // unparseable segments
	}
	for _, tok := range cases {
		t.Run(tok, func(t *testing.T) {
			_, err := VerifyJWT(secret, tok)
			if err == nil {
				t.Fatalf("expected error for %q", tok)
			}
			// Either ErrTokenMalformed itself or an error wrapping it is
			// acceptable — the wrapper case happens for unrecognised parse
			// errors that aren't covered by named sentinels.
			if !errors.Is(err, ErrTokenMalformed) && !errors.Is(err, ErrTokenInvalidSignature) {
				t.Errorf("expected malformed/invalid-sig error, got %v", err)
			}
		})
	}
}

// TestVerifyJWT_RejectsAlgNone defends against the classic "alg: none"
// downgrade attack. Even with no signature, the token must be rejected.
func TestVerifyJWT_RejectsAlgNone(t *testing.T) {
	// Hand-roll an alg=none token (golang-jwt won't sign one for us).
	// The structure is just a header+payload+empty-sig where the header
	// claims alg=none.
	header := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0"
	payload := "eyJ1c2VyX2lkIjoidSIsInJvbGUiOiJ1c2VyIiwiZW1haWwiOiJhQGIiLCJpc3MiOiJkdXJhZ3JhcGgtcGxhdGZvcm0ifQ"
	tok := header + "." + payload + "."

	_, err := VerifyJWT([]byte("secret"), tok)
	if err == nil {
		t.Fatal("alg=none token must be rejected")
	}
}

// TestVerifyJWT_MissingRequiredClaim checks the post-signature claim
// validation: even a properly-signed, non-expired, right-issuer token
// must carry user_id, role, email.
func TestVerifyJWT_MissingRequiredClaim(t *testing.T) {
	secret := []byte("secret")

	cases := []struct {
		name   string
		claims Claims
	}{
		{
			name: "missing user_id",
			claims: Claims{
				Role:  "user",
				Email: "a@b",
			},
		},
		{
			name: "missing role",
			claims: Claims{
				UserID: "u",
				Email:  "a@b",
			},
		},
		{
			name: "missing email",
			claims: Claims{
				UserID: "u",
				Role:   "user",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.claims.RegisteredClaims = jwt.RegisteredClaims{
				Issuer:    IssuerDuragraphPlatform,
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			}
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, tc.claims)
			signed, _ := tok.SignedString(secret)

			_, err := VerifyJWT(secret, signed)
			if !errors.Is(err, ErrTokenMissingClaim) {
				t.Errorf("expected ErrTokenMissingClaim, got %v", err)
			}
		})
	}
}

// TestNewVerifier_RejectsEmptySecret guards against a config oversight —
// running with AUTH_ENABLED=true and no JWT_SECRET should be a hard error
// at startup, not a runtime accept-everything.
func TestNewVerifier_RejectsEmptySecret(t *testing.T) {
	_, err := NewVerifier(nil)
	if err == nil {
		t.Error("nil secret must be rejected")
	}
	_, err = NewVerifier([]byte{})
	if err == nil {
		t.Error("empty secret must be rejected")
	}
}

// TestVerifier_Verify confirms the method is a faithful wrapper around
// VerifyJWT.
func TestVerifier_Verify(t *testing.T) {
	secret := []byte("secret")
	v, err := NewVerifier(secret)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	signed, err := IssueJWT(secret, "u", "a@b", "user", "", time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}

	claims, err := v.Verify(signed)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "u" {
		t.Errorf("user_id: got %q", claims.UserID)
	}
}

// TestIssueJWT_DropsLegacyClaims is a regression guard — earlier
// iterations of generateJWT included `name` and `provider` fields. The
// canonical Claims shape MUST NOT carry those, since the engine middleware
// does not consume them. Decoding the JWT payload as raw JSON ensures
// there are no stowaway fields.
func TestIssueJWT_DropsLegacyClaims(t *testing.T) {
	secret := []byte("secret")
	signed, err := IssueJWT(secret, "u", "a@b", "user", "t", time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}

	// Decode middle segment (payload) to inspect raw fields.
	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(parts))
	}
	// We don't need to decode base64 here — golang-jwt's parser already
	// validated structural correctness above. Just confirm the parsed
	// Claims struct doesn't surface name/provider, by checking
	// jwt.MapClaims via a re-parse.
	parsed, err := jwt.Parse(signed, func(_ *jwt.Token) (interface{}, error) {
		return secret, nil
	})
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("expected MapClaims, got %T", parsed.Claims)
	}
	if _, has := mapClaims["name"]; has {
		t.Error("name claim must not be present in canonical token")
	}
	if _, has := mapClaims["provider"]; has {
		t.Error("provider claim must not be present in canonical token")
	}
	// And the canonical claims SHOULD be present.
	for _, want := range []string{"user_id", "tenant_id", "role", "email", "iat", "exp", "iss"} {
		if _, has := mapClaims[want]; !has {
			t.Errorf("canonical claim %q missing", want)
		}
	}
}
