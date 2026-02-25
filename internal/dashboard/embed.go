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

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the /dashboard/ prefix for file lookups.
		path := strings.TrimPrefix(r.URL.Path, "/dashboard")
		if path == "" || path == "/" {
			path = "/index.html"
		}

		// Try to open the file to check if it exists.
		f, err := sub.Open(strings.TrimPrefix(path, "/"))
		if err != nil {
			// File doesn't exist — serve index.html for SPA routing.
			r.URL.Path = "/index.html"
			fileServer.ServeHTTP(w, r)
			return
		}
		_ = f.Close()

		// Serve the actual file.
		r.URL.Path = path
		fileServer.ServeHTTP(w, r)
	})
}
