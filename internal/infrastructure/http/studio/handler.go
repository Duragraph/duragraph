// Package studio serves the embedded Studio UI under /studio/* with
// SPA-style fallback (any unknown sub-path resolves to studio/index.html
// so client-side routing can take over).
//
// Studio is the developer/end-user UI for interacting with deployed
// agents — distinct from the dashboard, which is the operator/admin UI.
// The Studio bundle is opt-in: dev/serve only mounts it when --studio
// is passed AND studio/dist embeds non-placeholder content.
package studio

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Register mounts the Studio SPA at /studio/* on the supplied echo
// instance. Use a stripped sub-FS (rooted at studio/dist) — see
// duragraph.StudioFS().
func Register(e *echo.Echo, distFS fs.FS) {
	e.GET("/studio", func(c echo.Context) error {
		// Canonical redirect /studio → /studio/ so relative asset paths
		// in index.html resolve correctly. http.FileServer would do this
		// for us if we mounted at "/", but the studio prefix needs explicit
		// handling.
		return c.Redirect(http.StatusMovedPermanently, "/studio/")
	})
	e.GET("/studio/*", spaHandler(distFS))
}

func spaHandler(distFS fs.FS) echo.HandlerFunc {
	httpFS := http.FS(distFS)
	fileServer := http.StripPrefix("/studio", http.FileServer(httpFS))

	return func(c echo.Context) error {
		req := c.Request()
		// Path inside studio/dist (no /studio prefix).
		rel := strings.TrimPrefix(req.URL.Path, "/studio/")
		if rel == "" {
			rel = "index.html"
		}

		if f, err := distFS.Open(rel); err == nil {
			f.Close()
			fileServer.ServeHTTP(c.Response().Writer, req)
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		// SPA fallback: rewrite to /studio/ so the file server returns
		// studio/dist/index.html. Using /studio/index.html would trigger
		// http.FileServer's canonical-redirect behaviour.
		fallback := req.Clone(req.Context())
		fallback.URL.Path = "/studio/"
		fileServer.ServeHTTP(c.Response().Writer, fallback)
		return nil
	}
}
