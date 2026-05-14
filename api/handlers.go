// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func (app *application) sitesHandler(w http.ResponseWriter, r *http.Request) {
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

	sites, err := app.listSites(r.Context(), orgID)
	if err != nil {
		log.Printf("list sites: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load sites"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sites": sites})
}

func (app *application) siteByIDHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sites/")
	path = strings.TrimSpace(path)

	parts := strings.SplitN(path, "/", 2)
	siteID := parts[0]
	if siteID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid site id"})
		return
	}

	if len(parts) == 2 {
		switch strings.TrimRight(parts[1], "/") {
		case "deploys":
			app.listDeploysHandler(w, r, siteID)
		case "rollback":
			app.rollbackHandler(w, r, siteID)
		case "thumbnail":
			app.siteThumbnailHandler(w, r, siteID)
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

		orgID, _, err := app.requireSiteAccess(r.Context(), user, siteID)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		deleted, err := app.deleteSite(r.Context(), orgID, siteID)
		if err != nil {
			log.Printf("delete site: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete site"})
			return
		}
		if !deleted {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "site not found"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodPatch:
		app.updateSiteHandler(w, r, siteID)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) updateSiteHandler(w http.ResponseWriter, r *http.Request, siteID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	orgID, _, err := app.requireSiteAccess(r.Context(), user, siteID)
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
		update sites set is_public = $1, updated_at = now()
		where id = $2 and deleted_at is null and org_id = $3
	`, *payload.IsPublic, siteID, orgID)
	if err != nil {
		log.Printf("update site visibility: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update site"})
		return
	}
	if commandTag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "isPublic": *payload.IsPublic})
}
