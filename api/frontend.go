package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var frontendFS embed.FS

func serveFrontend(mux *http.ServeMux) {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return
	}

	entries, err := fs.ReadDir(distFS, ".")
	if err != nil || len(entries) == 0 {
		return
	}

	fileServer := http.FileServer(http.FS(distFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Don't serve frontend for API or content routes
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/~") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the exact file
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if _, err := fs.Stat(distFS, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for unmatched routes
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
