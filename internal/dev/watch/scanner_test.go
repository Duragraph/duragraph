package watch

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestScanGraphs(t *testing.T) {
	dir := t.TempDir()

	// File with the sentinel — should be picked up.
	mustWrite(t, filepath.Join(dir, "hello.py"), "from duragraph.python import Graph\n@Graph()\nclass H: pass\n")

	// File without the sentinel — should be skipped.
	mustWrite(t, filepath.Join(dir, "plain.py"), "print('hi')\n")

	// File with sentinel inside a subdir — should be picked up (recursive).
	subdir := filepath.Join(dir, "agents")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(subdir, "nested.py"), "@Graph(name='x')\nclass N: pass\n")

	// File in a hidden dir — should be skipped.
	hidden := filepath.Join(dir, ".cache")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(hidden, "h.py"), "@Graph()\nclass X: pass\n")

	// File in __pycache__ — should be skipped.
	pyc := filepath.Join(dir, "__pycache__")
	if err := os.Mkdir(pyc, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(pyc, "c.py"), "@Graph()\nclass C: pass\n")

	// Non-python file with the sentinel — should be skipped (wrong ext).
	mustWrite(t, filepath.Join(dir, "readme.txt"), "@Graph(\n")

	got, err := ScanGraphs(dir)
	if err != nil {
		t.Fatalf("ScanGraphs: %v", err)
	}
	sort.Strings(got)
	want := []string{
		filepath.Join(dir, "agents", "nested.py"),
		filepath.Join(dir, "hello.py"),
	}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("entry %d: expected %s, got %s", i, want[i], got[i])
		}
	}
}

func TestScanGraphsMissingDir(t *testing.T) {
	_, err := ScanGraphs(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestScanGraphsNotADir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.py")
	mustWrite(t, f, "@Graph()\n")
	_, err := ScanGraphs(f)
	if err == nil {
		t.Fatal("expected error for non-dir argument")
	}
}

func TestHasGraphDecorator(t *testing.T) {
	dir := t.TempDir()
	yes := filepath.Join(dir, "yes.py")
	no := filepath.Join(dir, "no.py")
	mustWrite(t, yes, "import x\n@Graph(name='a')\n")
	mustWrite(t, no, "import x\nclass X: pass\n")

	if ok, err := HasGraphDecorator(yes); err != nil || !ok {
		t.Fatalf("yes.py: ok=%v err=%v", ok, err)
	}
	if ok, err := HasGraphDecorator(no); err != nil || ok {
		t.Fatalf("no.py: ok=%v err=%v", ok, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
