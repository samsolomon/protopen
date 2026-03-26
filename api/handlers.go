package main

import (
	"log"
	"net/http"
	"strings"
)

func (app *application) projectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	projects, err := app.listProjects(r.Context(), user.Email)
	if err != nil {
		log.Printf("list projects: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load projects"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (app *application) projectByIDHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	path = strings.TrimSpace(path)

	parts := strings.SplitN(path, "/", 2)
	projectID := parts[0]
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid project id"})
		return
	}

	if len(parts) == 2 {
		switch strings.TrimRight(parts[1], "/") {
		case "deploys":
			app.listDeploysHandler(w, r, projectID)
		case "rollback":
			app.rollbackHandler(w, r, projectID)
		default:
			http.NotFound(w, r)
		}
		return
	}

	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	deleted, err := app.deleteProject(r.Context(), user.Email, projectID)
	if err != nil {
		log.Printf("delete project: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete project"})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
