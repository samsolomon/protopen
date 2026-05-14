// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func (app *application) requireAdmin(w http.ResponseWriter, r *http.Request) (sessionUser, bool) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return sessionUser{}, false
	}
	if !requireSession(user, w) {
		return sessionUser{}, false
	}
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return sessionUser{}, false
	}
	return user, true
}

type adminUser struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	Username        string     `json:"username"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt,omitempty"`
	CreatedAt       string     `json:"createdAt"`
	OrgID           *string    `json:"orgId,omitempty"`
}

func (app *application) adminUsersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, ok := app.requireAdmin(w, r); !ok {
		return
	}

	rows, err := app.db.Query(r.Context(), `
		SELECT u.id, u.email, u.name, u.username, u.email_verified_at, u.created_at,
		       o.id
		FROM users u
		LEFT JOIN LATERAL (
			SELECT o.id FROM org_members m
			JOIN organizations o ON o.id = m.org_id AND o.is_personal = true
			WHERE m.user_id = u.id
			LIMIT 1
		) o ON true
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		log.Printf("admin list users: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load users"})
		return
	}
	defer rows.Close()

	var users []adminUser
	for rows.Next() {
		var u adminUser
		var createdAt time.Time
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Username, &u.EmailVerifiedAt, &createdAt, &u.OrgID); err != nil {
			log.Printf("admin scan user: %v", err)
			continue
		}
		u.CreatedAt = relativeTime(createdAt)
		users = append(users, u)
	}

	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (app *application) adminUserByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, ok := app.requireAdmin(w, r); !ok {
		return
	}

	userID := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	userID = strings.TrimRight(userID, "/")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user ID is required"})
		return
	}

	tx, err := app.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete user"})
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := tx.Exec(r.Context(), `
		DELETE FROM organizations WHERE id IN (
			SELECT o.id FROM organizations o
			JOIN org_members m ON m.org_id = o.id
			WHERE m.user_id = $1 AND o.is_personal = true
		)
	`, userID); err != nil {
		log.Printf("admin delete user org: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete user"})
		return
	}

	tag, err := tx.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		log.Printf("admin delete user: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete user"})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete user"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type adminSite struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	UpdatedAt    string `json:"updatedAt"`
	DeployCount  int    `json:"deployCount"`
	StorageBytes int64  `json:"storageBytes"`
	OrgSlug      string `json:"orgSlug"`
	OrgName      string `json:"orgName"`
	IsPublic     bool   `json:"isPublic"`
	LiveURL      string `json:"liveUrl"`
}

func (app *application) adminSitesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, ok := app.requireAdmin(w, r); !ok {
		return
	}

	rows, err := app.db.Query(r.Context(), `
		SELECT p.id, p.name, p.slug, p.updated_at,
		       COUNT(d.id) as deploy_count,
		       COALESCE(SUM(d.size_bytes), 0) as storage_bytes,
		       o.slug, o.name, p.is_public
		FROM sites p
		JOIN organizations o ON o.id = p.org_id
		LEFT JOIN deploys d ON d.site_id = p.id
		WHERE p.deleted_at IS NULL
		GROUP BY p.id, o.slug, o.name
		ORDER BY p.updated_at DESC
		LIMIT 200
	`)
	if err != nil {
		log.Printf("admin list sites: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load sites"})
		return
	}
	defer rows.Close()

	var sites []adminSite
	for rows.Next() {
		var s adminSite
		var updatedAt time.Time
		if err := rows.Scan(&s.ID, &s.Name, &s.Slug, &updatedAt, &s.DeployCount, &s.StorageBytes, &s.OrgSlug, &s.OrgName, &s.IsPublic); err != nil {
			log.Printf("admin scan site: %v", err)
			continue
		}
		s.UpdatedAt = relativeTime(updatedAt)
		s.LiveURL = fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, s.OrgSlug, s.Slug)
		sites = append(sites, s)
	}

	writeJSON(w, http.StatusOK, map[string]any{"sites": sites})
}

func (app *application) adminSiteByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, ok := app.requireAdmin(w, r); !ok {
		return
	}

	siteID := strings.TrimPrefix(r.URL.Path, "/api/admin/sites/")
	siteID = strings.TrimRight(siteID, "/")
	if siteID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "site ID is required"})
		return
	}

	tag, err := app.db.Exec(r.Context(), `
		UPDATE sites SET deleted_at = now(), current_deploy_id = null, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, siteID)
	if err != nil {
		log.Printf("admin delete site: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete site"})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
