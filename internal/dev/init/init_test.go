package initpkg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	initpkg "github.com/duragraph/duragraph/internal/dev/init"
)

// TestTemplateRendering_FailsOnMissingKey is a unit-level guard for the
// `missingkey=error` option used inside Scaffold's text/template setup.
// It exists so a future refactor that drops the option (e.g. via an
// extracted helper) gets caught immediately. We test the option at the
// text/template level rather than via Scaffold because the embedded
// templates are intentionally typo-free; the package-internal contract
// is "every template parsed by this package uses missingkey=error".
func TestTemplateRendering_FailsOnMissingKey(t *testing.T) {
	tmpl, err := template.New("typo.tmpl").
		Option("missingkey=error").
		Parse(`hello {{.NotAField}}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	data := struct{ ProjectName string }{ProjectName: "x"}
	var sink strings.Builder
	if err := tmpl.Execute(&sink, data); err == nil {
		t.Fatalf("expected execute to fail on missing key, got output %q", sink.String())
	}
}

func TestListTemplates_ReturnsWhitelist(t *testing.T) {
	got := initpkg.ListTemplates()
	want := []string{"hello-world", "chatbot", "rag", "tool-use"}
	if len(got) != len(want) {
		t.Fatalf("ListTemplates() returned %d entries, want %d (%v)", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("ListTemplates()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestScaffold_HelloWorld_CreatesExpectedFiles(t *testing.T) {
	target := filepath.Join(t.TempDir(), "myproj")
	if err := initpkg.Scaffold(initpkg.Options{
		ProjectName: "myproj",
		Template:    "hello-world",
		TargetDir:   target,
	}); err != nil {
		t.Fatalf("Scaffold returned error: %v", err)
	}

	// Required files; none of these should be empty.
	required := []string{
		"duragraph.yaml",
		"pyproject.toml",
		"README.md",
		filepath.Join("agents", "hello.py"),
	}
	for _, rel := range required {
		full := filepath.Join(target, rel)
		info, err := os.Stat(full)
		if err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("expected %s to be a file, got directory", rel)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("expected %s to be non-empty", rel)
		}
		// .tmpl extension must be stripped on output.
		if strings.HasSuffix(full, ".tmpl") {
			t.Errorf("output file %s should not have .tmpl suffix", full)
		}
	}

	// No stray .tmpl files should leak into the output.
	walkErr := filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".tmpl") {
			t.Errorf("found .tmpl file in scaffold output: %s", path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk error: %v", walkErr)
	}
}

func TestScaffold_AllTemplatesProduceCoreFiles(t *testing.T) {
	for _, tmpl := range initpkg.ListTemplates() {
		tmpl := tmpl
		t.Run(tmpl, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "proj-"+tmpl)
			if err := initpkg.Scaffold(initpkg.Options{
				ProjectName: "proj-" + strings.ReplaceAll(tmpl, "-", ""),
				Template:    tmpl,
				TargetDir:   target,
			}); err != nil {
				t.Fatalf("Scaffold(%s) error: %v", tmpl, err)
			}
			for _, rel := range []string{"duragraph.yaml", "pyproject.toml", "README.md"} {
				if _, err := os.Stat(filepath.Join(target, rel)); err != nil {
					t.Errorf("[%s] missing %s: %v", tmpl, rel, err)
				}
			}
			// Each template must ship at least one .py under agents/.
			agentsDir := filepath.Join(target, "agents")
			entries, err := os.ReadDir(agentsDir)
			if err != nil {
				t.Fatalf("[%s] read agents/: %v", tmpl, err)
			}
			pyFound := false
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".py") {
					pyFound = true
					break
				}
			}
			if !pyFound {
				t.Errorf("[%s] no .py file in agents/", tmpl)
			}
		})
	}
}

func TestScaffold_RejectsBadTemplate(t *testing.T) {
	target := filepath.Join(t.TempDir(), "proj")
	err := initpkg.Scaffold(initpkg.Options{
		ProjectName: "proj",
		Template:    "does-not-exist",
		TargetDir:   target,
	})
	if err == nil {
		t.Fatal("expected error for unknown template, got nil")
	}
	if !strings.Contains(err.Error(), "unknown template") {
		t.Errorf("error %q does not mention 'unknown template'", err)
	}
}

func TestScaffold_RejectsBadProjectName(t *testing.T) {
	cases := []struct {
		name    string
		project string
	}{
		{"empty", ""},
		{"digit-start", "1proj"},
		{"dash-start", "-proj"},
		{"uppercase", "Proj"},
		{"underscore", "my_proj"},
		{"space", "my proj"},
		{"too-long", strings.Repeat("a", 65)},
		{"special", "proj!"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "out")
			err := initpkg.Scaffold(initpkg.Options{
				ProjectName: tc.project,
				Template:    "hello-world",
				TargetDir:   target,
			})
			if err == nil {
				t.Fatalf("expected error for project name %q, got nil", tc.project)
			}
		})
	}
}

func TestScaffold_RejectsNonEmptyTargetDir(t *testing.T) {
	target := t.TempDir()
	// Drop a sentinel file so the dir is non-empty.
	if err := os.WriteFile(filepath.Join(target, "preexisting"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := initpkg.Scaffold(initpkg.Options{
		ProjectName: "proj",
		Template:    "hello-world",
		TargetDir:   target,
	})
	if err == nil {
		t.Fatal("expected error for non-empty target dir, got nil")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("error %q does not mention 'non-empty'", err)
	}
}

func TestScaffold_AcceptsMissingTargetDir(t *testing.T) {
	// Missing path should be created.
	target := filepath.Join(t.TempDir(), "missing", "nested", "proj")
	if err := initpkg.Scaffold(initpkg.Options{
		ProjectName: "proj",
		Template:    "hello-world",
		TargetDir:   target,
	}); err != nil {
		t.Fatalf("Scaffold to missing dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "duragraph.yaml")); err != nil {
		t.Errorf("missing scaffolded file: %v", err)
	}
}

func TestScaffold_AcceptsEmptyExistingTargetDir(t *testing.T) {
	target := t.TempDir() // empty tempdir
	if err := initpkg.Scaffold(initpkg.Options{
		ProjectName: "proj",
		Template:    "hello-world",
		TargetDir:   target,
	}); err != nil {
		t.Fatalf("Scaffold into empty dir: %v", err)
	}
}

func TestScaffold_TemplateRenderingSubstitutesProjectName(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	const projectName = "cool-proj"
	if err := initpkg.Scaffold(initpkg.Options{
		ProjectName: projectName,
		Template:    "hello-world",
		TargetDir:   target,
	}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	for _, rel := range []string{"pyproject.toml", "README.md", "duragraph.yaml"} {
		body, err := os.ReadFile(filepath.Join(target, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(body), projectName) {
			t.Errorf("%s does not contain project name %q. Body:\n%s", rel, projectName, body)
		}
		// No raw `{{.ProjectName}}` markers should remain.
		if strings.Contains(string(body), "{{") {
			t.Errorf("%s still contains template markers (`{{`):\n%s", rel, body)
		}
	}
}

func TestScaffold_RejectsEmptyTargetDir(t *testing.T) {
	err := initpkg.Scaffold(initpkg.Options{
		ProjectName: "proj",
		Template:    "hello-world",
		TargetDir:   "",
	})
	if err == nil {
		t.Fatal("expected error for empty TargetDir, got nil")
	}
}
