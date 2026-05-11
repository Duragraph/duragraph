// Package duragraph exports the embedded dashboard filesystem so the
// HTTP server can serve the UI from the same binary.
//
// dashboard/dist must live at the module root: Go's //go:embed directive
// can only embed files within or below the directory containing the
// directive. Splitting it under internal/ would break Vite/pnpm's
// expectation that `dist/` sits next to package.json.
//
// Studio used to ship as a second //go:embed (studio/dist) and a
// separate /studio/* mount. As of the studio-into-dashboard merge,
// studio's developer-UI surface (chat playground, workflow builder,
// deployments, run inspector) is folded into dashboard's TanStack
// Router tree at /playground, /builder, /deployments, /inspector. No
// separate embed remains.
package duragraph

import (
	"embed"
	"io/fs"
)

//go:embed all:dashboard/dist
var dashboardDist embed.FS

// DashboardFS returns an fs.FS rooted at dashboard/dist for the
// embedded React UI. Served at "/" by the dashboard handler.
func DashboardFS() (fs.FS, error) {
	return fs.Sub(dashboardDist, "dashboard/dist")
}
