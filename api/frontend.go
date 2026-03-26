package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var frontendFS embed.FS

func serveFrontend(mux *http.ServeMux, frontendOrigin string) {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return
	}

	// Check for a built frontend
	if _, err := fs.Stat(distFS, "index.html"); err != nil {
		// No built frontend — redirect to the frontend dev server
		if frontendOrigin != "" {
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/~") {
					http.NotFound(w, r)
					return
				}
				target := frontendOrigin + r.URL.RequestURI()
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			})
		}
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
