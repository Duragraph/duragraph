package watch

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// graphSentinel is the substring we look for in candidate Python files
// to decide whether to spawn a worker for them. Using a substring match
// (rather than parsing the AST) is intentional: the scan must run on
// every directory walk and on every fsnotify Write event, so it has to
// be cheap. False positives are bounded — the worst case is spawning a
// worker for a file that imports `@Graph(` in a comment, which then
// fails to register and just exits. False negatives matter more, but
// any normal Python source that defines a graph will contain the
// literal string.
var graphSentinel = []byte("@Graph(")

// HasGraphDecorator reports whether the file at path appears to define
// a graph (contains the @Graph( substring). Returns (false, nil) for
// files that exist but lack the sentinel; returns an error only on I/O
// failure.
func HasGraphDecorator(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return bytes.Contains(data, graphSentinel), nil
}

// ScanGraphs walks dir recursively and returns absolute paths of every
// `.py` file containing the @Graph( sentinel. Hidden directories (names
// starting with '.') and __pycache__ are skipped — they're never source
// the operator wants supervised, and walking them wastes I/O.
//
// Returns a non-nil (possibly empty) slice on success. Returns an error
// only if the root dir can't be read; per-file read errors are skipped
// with a best-effort policy so one unreadable file doesn't poison the
// whole scan.
func ScanGraphs(dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}

	var found []string
	walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Don't abort the whole walk on a single permission denied;
			// just skip the entry.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			// Skip hidden + __pycache__. Don't skip the root itself
			// even if its name starts with '.' — the operator may have
			// pointed --watch at a hidden dir intentionally.
			if path != abs && (name[0] == '.' || name == "__pycache__" || name == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".py" {
			return nil
		}
		ok, readErr := HasGraphDecorator(path)
		if readErr != nil {
			return nil
		}
		if ok {
			found = append(found, path)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return found, nil
}
