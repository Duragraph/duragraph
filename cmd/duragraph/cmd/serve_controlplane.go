package cmd

// Wiring for the rebuilt control plane (controlplane/…) into the shipped
// binary.
//
// Until this file existed, `go list -deps ./cmd/duragraph` contained no
// controlplane package at all: the rebuild was reachable only from tests.
// GoReleaser builds ./cmd/duragraph and nothing else, so every release
// shipped the legacy internal/ stack while the rebuild rode along as dead
// weight in the repo.
//
// WHY THIS IS OPT-IN RATHER THAN A SWAP. The two stacks do not serve the
// same surface. Comparing routes, the rebuild is missing sixteen that the
// legacy server answers today, including:
//
//	POST /api/auth/register, POST /api/auth/login   (password auth —
//	                                                 shipped and documented)
//	GET  /health                                    (container/k8s probes)
//	GET  /assistants/{id}/schemas, …/subgraphs      (declared in api.d2)
//	POST /mcp
//	the legacy poll-based worker protocol
//
// Flipping the default would silently delete those. So `--control-plane`
// selects, the default stays legacy, and the gap is closed before the
// default moves. Making it reachable is worth doing now regardless: an
// opt-in path can be deployed, exercised, and fixed, whereas unreachable
// code cannot.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	cpserver "github.com/duragraph/duragraph/controlplane/server"
	"github.com/duragraph/duragraph/internal/config"
)

// controlPlaneFlag selects the implementation `serve` runs.
var controlPlaneFlag string

const (
	controlPlaneLegacy = "legacy"
	controlPlaneV2     = "v2"
)

func init() {
	serveCmd.Flags().StringVar(&controlPlaneFlag, "control-plane", "",
		`which control-plane implementation to run: "legacy" (default) or "v2".
"v2" is the rebuilt control plane. It is not yet a drop-in replacement —
password auth, /health, /mcp and the assistant schema/subgraph endpoints
are not implemented there yet. Also settable via DURAGRAPH_CONTROL_PLANE.`)
}

// selectedControlPlane resolves the flag, falling back to the environment
// and then to legacy. An unrecognised value is an error rather than a
// silent fallback: someone who typed --control-plane=V2 and got the legacy
// stack would have no way to tell.
func selectedControlPlane() (string, error) {
	v := controlPlaneFlag
	if v == "" {
		v = os.Getenv("DURAGRAPH_CONTROL_PLANE")
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", controlPlaneLegacy:
		return controlPlaneLegacy, nil
	case controlPlaneV2:
		return controlPlaneV2, nil
	default:
		return "", fmt.Errorf(
			"unknown --control-plane %q: expected %q or %q",
			v, controlPlaneLegacy, controlPlaneV2)
	}
}

// runServeV2 builds the rebuilt control plane from the same environment
// configuration the legacy path reads, so switching implementations does
// not also mean rewriting a deployment's config.
func runServeV2(ctx context.Context, cfg *config.Config) error {
	dsn := postgresDSN(cfg, cfg.Database.Database)

	// The platform database holds users and tenants. The rebuild tolerates
	// its absence (the auth/admin/platform groups then answer 500), so this
	// is passed through rather than required — a single-tenant bootstrap
	// should not need it.
	platformDSN := os.Getenv("DURAGRAPH_PLATFORM_DSN")

	// LISTEN/NOTIFY needs a session-affine connection, so it must not go
	// through PgBouncer. RelayDSN is the escape hatch for pooler-fronted
	// deployments; without one, the ordinary DSN is right for dev and for
	// any deploy that talks to Postgres directly.
	listenerDSN := cfg.Database.RelayDSN
	if listenerDSN == "" {
		listenerDSN = dsn
	}

	scfg := cpserver.Config{
		Addr:        cfg.ServerAddr(),
		TenantDSN:   dsn,
		PlatformDSN: platformDSN,
		NATSURL:     cfg.NATS.URL,
		ListenerDSN: listenerDSN,
		// Relays publish the event stream. They need JetStream, so they
		// follow NATS being configured at all.
		Relays: cfg.NATS.URL != "",
		// Migrations are embedded in the binary and idempotent, so running
		// them on every boot is safe and makes first boot work with no
		// separate step.
		Migrate: true,
	}

	slog.Info("starting rebuilt control plane",
		"addr", scfg.Addr,
		"nats", scfg.NATSURL != "",
		"platform_db", scfg.PlatformDSN != "",
	)

	srv, err := cpserver.New(ctx, scfg)
	if err != nil {
		return fmt.Errorf("control plane: %w", err)
	}
	return srv.Run(ctx)
}

// postgresDSN renders the connection string for a database on the
// configured host. Kept here so both serve paths agree on how the
// discrete DB_* settings become a DSN.
func postgresDSN(cfg *config.Config, database string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Host, cfg.Database.Port,
		database, cfg.Database.SSLMode,
	)
}
