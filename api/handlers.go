package main

import (
	"encoding/json"
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

	orgID, _, err := resolveOrgFromParam(user, r.URL.Query().Get("org"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	projects, err := app.listProjects(r.Context(), orgID)
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

	switch r.Method {
	case http.MethodDelete:
		user, err := app.requireSessionUser(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if !requireSession(user, w) {
			return
		}

		orgID, _, err := app.requireProjectAccess(r.Context(), user, projectID)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		deleted, err := app.deleteProject(r.Context(), orgID, projectID)
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

	case http.MethodPatch:
		app.updateProjectHandler(w, r, projectID)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) updateProjectHandler(w http.ResponseWriter, r *http.Request, projectID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !requireSession(user, w) {
		return
	}

	orgID, _, err := app.requireProjectAccess(r.Context(), user, projectID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	var payload struct {
		IsPublic *bool `json:"isPublic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if payload.IsPublic == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "isPublic is required"})
		return
	}

	commandTag, err := app.db.Exec(r.Context(), `
		update projects set is_public = $1, updated_at = now()
		where id = $2 and deleted_at is null and org_id = $3
	`, *payload.IsPublic, projectID, orgID)
	if err != nil {
		log.Printf("update project visibility: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update project"})
		return
	}
	if commandTag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "isPublic": *payload.IsPublic})
}
