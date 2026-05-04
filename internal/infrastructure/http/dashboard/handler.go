// Package dashboard serves the embedded React dashboard with SPA-style
// fallback so client-side routes resolve to index.html.
package dashboard

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Register mounts the dashboard at the catch-all GET route. API and system
// routes (/api/*, /health, /metrics, /ok, /info) must already be registered;
// Echo's router prioritises exact matches over the wildcard.
func Register(e *echo.Echo, distFS fs.FS) {
	e.GET("/*", spaHandler(distFS))
}

func spaHandler(distFS fs.FS) echo.HandlerFunc {
	httpFS := http.FS(distFS)
	fileServer := http.FileServer(httpFS)

	return func(c echo.Context) error {
		req := c.Request()
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if f, err := distFS.Open(path); err == nil {
			f.Close()
			fileServer.ServeHTTP(c.Response().Writer, req)
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		// SPA fallback: any unknown path serves index.html so client-side
		// routing (TanStack Router) can take over. Rewrite the request URL
		// to "/" — using "/index.html" would trigger http.FileServer's
		// canonical-redirect behaviour (301 to "/").
		fallback := req.Clone(req.Context())
		fallback.URL.Path = "/"
		fileServer.ServeHTTP(c.Response().Writer, fallback)
		return nil
	}
}
