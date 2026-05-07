package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Version information (set by GoReleaser / -ldflags at build time).
// Kept here under the new cobra package; the legacy copy in
// cmd/server has been deleted and the back-compat shim in
// cmd/server/main.go now defers to this command tree.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

// VersionInfo describes the binary build for human + JSON output.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	BuiltBy   string `json:"built_by"`
	GoVersion string `json:"go_version"`
}

// GetVersion returns the version metadata baked into the binary.
func GetVersion() VersionInfo {
	return VersionInfo{
		Version:   version,
		Commit:    commit,
		Date:      date,
		BuiltBy:   builtBy,
		GoVersion: runtime.Version(),
	}
}

// String renders the human-readable version block (matches the
// previous cmd/server output format).
func (v VersionInfo) String() string {
	return fmt.Sprintf("DuraGraph %s\nCommit: %s\nBuilt: %s by %s\nGo: %s",
		v.Version, v.Commit, v.Date, v.BuiltBy, v.GoVersion)
}

// ShortVersion returns just the version number.
func (v VersionInfo) ShortVersion() string {
	return v.Version
}

var versionJSON bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print binary version and build metadata",
	Long: `Print the binary version, git commit, build date, builder, and Go runtime.

With --json the output is a single JSON object (suitable for piping into jq).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := GetVersion()
		if versionJSON {
			b, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal version: %w", err)
			}
			fmt.Println(string(b))
			return nil
		}
		fmt.Println(v.String())
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "emit version metadata as JSON")
	rootCmd.AddCommand(versionCmd)
}
