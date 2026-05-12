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

// Register mounts the dashboard at a catch-all route that accepts ALL
// methods. API and system routes (/api/*, /health, /metrics, /ok, /info)
// must already be registered; Echo's router prioritises exact matches over
// the wildcard.
//
// Why Any and not GET: registering this as e.GET caused unmatched POST /api/*
// requests (e.g. when password auth is disabled and the dashboard still
// tries to POST /api/auth/login) to return 405 Method Not Allowed because
// Echo's router matched the path against the GET wildcard but rejected the
// method *before* any handler ran. Registering for Any lets the handler run
// for every method, where it can return a clean 404 for /api/* paths that
// have no real registered route.
func Register(e *echo.Echo, distFS fs.FS) {
	e.Any("/*", spaHandler(distFS))
}

func spaHandler(distFS fs.FS) echo.HandlerFunc {
	httpFS := http.FS(distFS)
	fileServer := http.FileServer(httpFS)

	return func(c echo.Context) error {
		req := c.Request()

		// Never absorb /api/* paths into the SPA fallback. When an API
		// route is conditionally disabled (e.g. POST /api/auth/login
		// is only registered when AUTH_PASSWORD_ENABLED=true), an
		// unmatched request against /api/* would otherwise hit this
		// catch-all and either serve index.html (for GET) or return 405
		// (for POST/PUT/DELETE), both of which are confusing. Returning
		// ErrNotFound here lets the engine's error middleware respond
		// with a clean 404 + JSON body that callers can parse.
		//
		// Health / metrics / system endpoints (/health, /ok, /info,
		// /metrics) and real API routes are registered with exact paths
		// and reach their handlers before this catch-all fires, so this
		// guard only filters genuinely-unmatched /api/* requests.
		if strings.HasPrefix(req.URL.Path, "/api/") {
			return echo.ErrNotFound
		}

		// Non-API, non-GET requests against the SPA make no sense (the
		// dashboard is HTML/JS/CSS served via GET; state changes go to
		// /api/*). Return 405 instead of trying to read static assets
		// with the wrong method.
		if req.Method != http.MethodGet {
			return echo.ErrMethodNotAllowed
		}

		path := strings.TrimPrefix(req.URL.Path, "/")
		// Trim trailing slash before fs.Open — io/fs treats "studio/"
		// as an invalid name and returns ErrInvalid rather than ErrNotExist,
		// which would otherwise propagate as a 500 for any URL ending in "/"
		// that doesn't map to a real subtree (e.g. /studio/ post-merge).
		path = strings.TrimSuffix(path, "/")
		if path == "" {
			path = "index.html"
		}

		if f, err := distFS.Open(path); err == nil {
			f.Close()
			fileServer.ServeHTTP(c.Response().Writer, req)
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrInvalid) {
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
