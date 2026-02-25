// Package dashboard embeds the pre-built React dashboard assets and provides
// an http.Handler to serve them with proper SPA fallback routing.
package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded dashboard.
// It handles SPA routing by falling back to index.html for any path
// that doesn't match a static file.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("dashboard: failed to access embedded dist: " + err.Error())
	}

	// Pre-read index.html for direct serving. Go's http.FileServer redirects
	// "/index.html" to "./" (clean URL convention), which causes an infinite
	// redirect loop when mounted at a sub-path like /dashboard/. Serving the
	// bytes directly avoids this.
	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic("dashboard: missing index.html in embedded dist: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the /dashboard/ prefix for file lookups.
		path := strings.TrimPrefix(r.URL.Path, "/dashboard")
		if path == "" || path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexHTML)
			return
		}

		// Try to open the file to check if it exists.
		f, err := sub.Open(strings.TrimPrefix(path, "/"))
		if err != nil {
			// File doesn't exist — serve index.html for SPA routing.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexHTML)
			return
		}
		_ = f.Close()

		// Serve the actual static file (JS, CSS, images).
		r.URL.Path = path
		fileServer.ServeHTTP(w, r)
	})
}
