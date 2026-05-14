// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var frontendFS embed.FS

func serveFrontend(mux *http.ServeMux, frontendOrigin string, contentHandler ...http.Handler) {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return
	}

	var content http.Handler
	if len(contentHandler) > 0 {
		content = contentHandler[0]
	}

	serveContent := func(w http.ResponseWriter, r *http.Request) {
		if content != nil {
			content.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
	}

	// Check for a built frontend
	if _, err := fs.Stat(distFS, "index.html"); err != nil {
		// No built frontend — redirect to the frontend dev server
		if frontendOrigin != "" {
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					http.NotFound(w, r)
					return
				}
				if strings.HasPrefix(r.URL.Path, "/~") {
					serveContent(w, r)
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
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/~") {
			serveContent(w, r)
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
