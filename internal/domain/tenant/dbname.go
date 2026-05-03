package tenant

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// dbNameRegex enforces the canonical tenant DB name format: the literal
// prefix "tenant_" followed by exactly 32 lowercase hex characters (the
// UUID with hyphens stripped). Total length 39 chars, well under PG's
// 63-char identifier limit, and within [a-z0-9_] so DDL string-interpolation
// is safe when combined with ValidateDBName.
var dbNameRegex = regexp.MustCompile(`^tenant_[a-f0-9]{32}$`)

// DBName returns the deterministic Postgres database name for tenantID.
// Format: "tenant_" + 32 hex chars of UUID with hyphens stripped.
// Per duragraph-spec models/entities.yml#tenants derivation rule.
func DBName(tenantID string) (string, error) {
	u, err := uuid.Parse(tenantID)
	if err != nil {
		return "", fmt.Errorf("invalid tenant id: %w", err)
	}
	return "tenant_" + strings.ReplaceAll(u.String(), "-", ""), nil
}

// ValidateDBName checks that name matches the canonical derived format.
// Defense-in-depth before any DDL string interpolation — Postgres can't
// parameterize identifiers, so callers building DDL must validate first.
func ValidateDBName(name string) error {
	if !dbNameRegex.MatchString(name) {
		return fmt.Errorf("invalid tenant db name: %s", name)
	}
	return nil
}
