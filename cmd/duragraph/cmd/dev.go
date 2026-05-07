package cmd

import "github.com/spf13/cobra"

// devCmd is the zero-config dev mode entrypoint. Stub for now —
// embedded postgres + nats + watch mode wiring lands in phases 2-5
// of the v0.7 single-binary DX track (binary-modes.yml § migration).
var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Run duragraph with embedded postgres, nats, and watch mode (stub — not yet implemented)",
	Long: `Zero-config dev mode: embedded Postgres + embedded NATS + dashboard
+ optional Studio + worker watch mode.

Not yet implemented. Tracking phases 2-5 of binary-modes.yml § migration.phasing.`,
	RunE: notYetImplemented("dev"),
}

func init() {
	rootCmd.AddCommand(devCmd)
}
