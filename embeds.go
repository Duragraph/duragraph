// Package duragraph exports the embedded dashboard + Studio filesystems
// so the HTTP server can serve both UIs from the same binary.
//
// Both files have to live at the module root: Go's //go:embed directive
// can only embed files within or below the directory containing the
// directive, and dashboard/dist + studio/dist are at the module root.
// Splitting them into per-component sub-packages would force the dist
// directories to live under internal/, which breaks the Vite/pnpm
// build conventions for those frontends.
package duragraph

import (
	"embed"
	"io/fs"
)

//go:embed all:dashboard/dist
var dashboardDist embed.FS

//go:embed all:studio/dist
var studioDist embed.FS

// DashboardFS returns an fs.FS rooted at dashboard/dist for the
// embedded dashboard UI (operator/admin surface). Always available;
// the dashboard is served at "/" by default.
func DashboardFS() (fs.FS, error) {
	return fs.Sub(dashboardDist, "dashboard/dist")
}

// StudioFS returns an fs.FS rooted at studio/dist for the embedded
// Studio UI (developer/end-user surface). Mounted at /studio/* only
// when the operator opts in via `duragraph dev --studio` or
// DURAGRAPH_DEV_STUDIO=true on `duragraph serve`.
func StudioFS() (fs.FS, error) {
	return fs.Sub(studioDist, "studio/dist")
}
