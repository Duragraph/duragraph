package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// notYetImplemented returns a RunE that prints the standard
// "not yet implemented" error and exits non-zero. Used by every
// stub subcommand in this PR (dev, init, migrate, runs *, events *,
// studio *) so the message is uniform and the spec reference is
// only spelled once.
//
// The stub bodies will be filled in by follow-up PRs in the v0.7
// single-binary DX track — see duragraph-spec/backend/binary-modes.yml
// § subcommands and § migration.phasing for the per-phase scope.
func notYetImplemented(name string) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf(
			"%s: not yet implemented — see duragraph-spec/backend/binary-modes.yml",
			name,
		)
	}
}
