// Package initpkg scaffolds new duragraph projects from embedded templates.
//
// The Go package name is `initpkg` rather than `init` because `init` is a
// special identifier (every Go package has implicit `init()` functions),
// and using it as a package name shadows that keyword inside callers.
// The on-disk directory is still `internal/dev/init/` to match the
// "duragraph init" subcommand it implements.
//
// Implements binary-modes.yml § subcommands.duragraph_init (v0.7 Phase 6
// of the single-binary DX track). Templates are embedded at build time via
// //go:embed so the binary is self-contained — no GitHub fetch at runtime.
//
// Each template is a minimal, runnable scaffold mirroring the canonical
// example in duragraph-examples/python/. The intent is "starter you'd
// edit", not "production reference" — keep templates small.
//
// Files ending in `.tmpl` are rendered through text/template with the
// project name available as `{{.ProjectName}}`. The `.tmpl` extension is
// stripped on output. All other files are copied verbatim. Directory
// structure under `templates/<name>/` is preserved 1:1 in the target.
package initpkg

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

// templates holds the embedded scaffold sources. The `all:` prefix is
// required so dotfiles and underscored files (none today, but future
// .gitignore etc.) get included.
//
//go:embed all:templates
var templates embed.FS

// Options controls a single Scaffold invocation.
type Options struct {
	// ProjectName is the user-supplied identifier for the new project.
	// Validated as PEP-503-ish: lowercase letters/digits/dashes, must
	// not start with a digit or dash, ≤64 chars. Used in rendered files
	// (pyproject name, README title, duragraph.yaml project.name).
	ProjectName string

	// Template selects the scaffold variant. Must be one of the values
	// returned by ListTemplates(). Defaults are the caller's job (the
	// CLI defaults to "hello-world").
	Template string

	// TargetDir is the absolute directory the scaffold is written into.
	// May be a non-existent path (will be created) or an existing empty
	// directory. Non-empty existing directories are rejected.
	TargetDir string
}

// projectNamePattern is the validation regex for ProjectName: lowercase
// letters/digits/dashes, must start with a letter, ≤64 chars total.
// Underscores are intentionally rejected — the value flows into a
// pyproject.toml `name` field, where PEP-503 normalizes underscores to
// dashes anyway, so we just reject upfront for clarity.
var projectNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// ListTemplates returns the whitelist of supported template names, in
// stable order. The CLI uses this for both --template validation and
// the help string.
func ListTemplates() []string {
	return []string{"hello-world", "chatbot", "rag", "tool-use"}
}

// Scaffold writes a new project tree to opts.TargetDir using the
// embedded template selected by opts.Template. Returns a clear error on
// any validation failure or filesystem mishap; partial scaffolds are
// NOT cleaned up automatically (the caller can `rm -rf` if needed —
// matching `cargo new` / `npm init` behaviour).
func Scaffold(opts Options) error {
	if err := validateProjectName(opts.ProjectName); err != nil {
		return err
	}
	if err := validateTemplate(opts.Template); err != nil {
		return err
	}
	if opts.TargetDir == "" {
		return errors.New("target directory must not be empty")
	}
	if err := validateTargetDir(opts.TargetDir); err != nil {
		return err
	}

	if err := os.MkdirAll(opts.TargetDir, 0o755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	root := path("templates", opts.Template)
	data := struct{ ProjectName string }{ProjectName: opts.ProjectName}

	walkErr := fs.WalkDir(templates, root, func(srcPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Compute the path inside the project tree by stripping the
		// embed prefix `templates/<template>/`.
		rel, err := filepath.Rel(root, srcPath)
		if err != nil {
			return fmt.Errorf("compute relative path for %q: %w", srcPath, err)
		}
		if rel == "." {
			// Root of the embedded template — TargetDir already exists.
			return nil
		}

		dstPath := filepath.Join(opts.TargetDir, strings.TrimSuffix(rel, ".tmpl"))

		if d.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return fmt.Errorf("mkdir %q: %w", dstPath, err)
			}
			return nil
		}

		raw, err := templates.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read embedded template %q: %w", srcPath, err)
		}

		var out []byte
		if strings.HasSuffix(srcPath, ".tmpl") {
			tmpl, err := template.New(filepath.Base(srcPath)).Parse(string(raw))
			if err != nil {
				return fmt.Errorf("parse template %q: %w", srcPath, err)
			}
			var buf strings.Builder
			if err := tmpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("render template %q: %w", srcPath, err)
			}
			out = []byte(buf.String())
		} else {
			out = raw
		}

		if err := os.WriteFile(dstPath, out, 0o644); err != nil {
			return fmt.Errorf("write %q: %w", dstPath, err)
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	return nil
}

// validateProjectName enforces the PEP-503-ish naming rule documented
// on Options.ProjectName. The error message is shown verbatim to the
// user, so it explains the rule rather than just saying "invalid".
func validateProjectName(name string) error {
	if name == "" {
		return errors.New("project name must not be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("project name %q is longer than 64 characters", name)
	}
	if !projectNamePattern.MatchString(name) {
		return fmt.Errorf(
			"project name %q is invalid: must start with a lowercase letter and "+
				"contain only lowercase letters, digits, and dashes",
			name,
		)
	}
	return nil
}

// validateTemplate checks that the requested template is in the
// whitelist. Phrased as "unknown template" so the error reads like
// `cargo new --vcs <bad>` ("invalid value …").
func validateTemplate(name string) error {
	for _, t := range ListTemplates() {
		if t == name {
			return nil
		}
	}
	return fmt.Errorf(
		"unknown template %q. Supported: %s",
		name,
		strings.Join(ListTemplates(), ", "),
	)
}

// validateTargetDir rejects an existing non-empty directory; missing
// paths and empty existing directories are both accepted.
func validateTargetDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect target directory %q: %w", dir, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("target directory %q exists and is non-empty", dir)
	}
	return nil
}

// path joins embed.FS path segments. We deliberately use forward
// slashes (not filepath.Join) because embed.FS paths are always
// slash-separated, even on Windows.
func path(parts ...string) string {
	return strings.Join(parts, "/")
}
