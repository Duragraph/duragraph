// Package db carries the control-plane's SQL migrations as embedded
// files.
//
// They are embedded rather than read from disk because the shipped
// artifact is a single binary. `controlplane/server` previously defaulted
// to reading "controlplane/db/migrations" relative to the process working
// directory — a path that exists in a source checkout and nowhere else, so
// an installed binary (Homebrew, a container, anything run from $HOME)
// could not migrate at all. Tests never noticed: they set Migrate:false and
// applied the SQL themselves.
//
// go:embed only reads files at or below the package directory, which is
// why this file lives at controlplane/db rather than next to the server.
package db

import (
	"embed"
	"io/fs"
)

//go:embed migrations
var migrationsFS embed.FS

// FS returns the migration tree rooted so that "tenant" and "platform"
// are its top-level directories — the same shape os.DirFS over
// controlplane/db/migrations gives, so callers can treat an embedded and
// an on-disk tree identically.
func FS() fs.FS {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		// Unreachable: the embed directive above guarantees the path.
		// A panic here means the tree was restructured without updating
		// this file, which is a build-time mistake, not a runtime one.
		panic("controlplane/db: migrations subtree missing: " + err.Error())
	}
	return sub
}
