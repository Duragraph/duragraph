package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	initpkg "github.com/duragraph/duragraph/internal/dev/init"
	"github.com/spf13/cobra"
)

// initCmd scaffolds a new duragraph project from an embedded template.
//
// Implements binary-modes.yml § subcommands.duragraph_init (v0.7 Phase 6
// of the single-binary DX track). Templates ship inside the binary via
// //go:embed in internal/dev/init — the scaffold operation is offline.
//
// The duragraph.yaml shape written into the new project is invented by
// this PR (the spec only says "graph paths, worker config" without
// prescribing fields). A future spec PR can codify the schema; until
// then the format is intentionally minimal.
var initCmd = &cobra.Command{
	Use:   "init <project-name>",
	Short: "Scaffold a new duragraph project",
	Long: `Create a new duragraph project directory with starter agent code,
project config, and run instructions.

Templates: hello-world | chatbot | rag | tool-use

By default the project is scaffolded into ./<project-name>; pass --dir
to override. The target directory must not exist or must be empty.`,
	Args: cobra.ExactArgs(1),
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	name := args[0]
	tmpl, _ := cmd.Flags().GetString("template")
	target, _ := cmd.Flags().GetString("dir")

	if target == "" {
		// Default to ./<project-name> resolved against PWD so the
		// printed message can show an absolute path back to the user.
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		target = filepath.Join(cwd, name)
	} else if !filepath.IsAbs(target) {
		// Resolve relative --dir against PWD so the package's "absolute
		// path" requirement is satisfied without surprising the user.
		abs, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("resolve --dir %q: %w", target, err)
		}
		target = abs
	}

	if err := initpkg.Scaffold(initpkg.Options{
		ProjectName: name,
		Template:    tmpl,
		TargetDir:   target,
	}); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Created %s/ from %q template.\n", target, tmpl)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Next steps:")
	fmt.Fprintf(out, "  cd %s\n", filepath.Base(target))
	fmt.Fprintln(out, "  uv sync")
	fmt.Fprintln(out, "  duragraph dev --watch ./agents")
	return nil
}

func init() {
	initCmd.Flags().StringP("template", "t", "hello-world",
		fmt.Sprintf("Template to scaffold from (%s)",
			strings.Join(initpkg.ListTemplates(), "|")))
	initCmd.Flags().StringP("dir", "d", "",
		"Target directory (default ./<project-name>)")
	rootCmd.AddCommand(initCmd)
}
