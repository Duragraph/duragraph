// Package duragraph exports the embedded dashboard filesystem so the HTTP
// server can serve the React UI from the same binary.
package duragraph

import (
	"embed"
	"io/fs"
)

//go:embed all:dashboard/dist
var dashboardDist embed.FS

// DashboardFS returns an fs.FS rooted at dashboard/dist so callers don't have
// to know about the embed prefix.
func DashboardFS() (fs.FS, error) {
	return fs.Sub(dashboardDist, "dashboard/dist")
}
