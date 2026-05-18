// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"errors"
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

	// ?author=me is the only currently supported value; resolve it to the
	// caller's id. Other values are silently ignored so future expansions
	// (e.g. author=<username>) don't break older clients.
	opts := listSitesOpts{}
	if r.URL.Query().Get("author") == "me" {
		opts.AuthorUserID = user.ID
	}

	sites, err := app.listSites(r.Context(), orgID, opts)
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
		case "duplicate":
			app.duplicateSiteHandler(w, r, siteID)
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

		orgID, _, err := app.requireSiteMutate(r.Context(), user, siteID)
		if err != nil {
			writeJSON(w, statusForSiteMutateError(err), map[string]string{"error": err.Error()})
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

	orgID, _, err := app.requireSiteMutate(r.Context(), user, siteID)
	if err != nil {
		writeJSON(w, statusForSiteMutateError(err), map[string]string{"error": err.Error()})
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

// duplicateSiteHandler clones a site within the same org and attributes the
// new copy to the calling user. Any org member can duplicate; this is the
// non-destructive escape hatch when teammates can't directly modify a site
// they don't own.
func (app *application) duplicateSiteHandler(w http.ResponseWriter, r *http.Request, sourceSiteID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// Read-only access (org membership) is sufficient to clone.
	if _, _, err := app.requireSiteAccess(r.Context(), user, sourceSiteID); err != nil {
		status := http.StatusForbidden
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	result, err := app.duplicateSite(r.Context(), sourceSiteID, user.ID)
	if err != nil {
		log.Printf("duplicate site: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not duplicate site"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"site": result})
}

// statusForSiteMutateError maps the three failure modes of requireSiteMutate
// onto HTTP status codes: 404 for missing sites, 403 for everything else (not
// a member, or member-but-not-creator-or-admin).
func statusForSiteMutateError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, ErrSiteMutateForbidden) {
		return http.StatusForbidden
	}
	if strings.Contains(err.Error(), "not found") {
		return http.StatusNotFound
	}
	return http.StatusForbidden
}
