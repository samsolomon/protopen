// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// requireOrgAdminOrInstanceAdmin gates org-membership write endpoints. Instance
// admins (email in ADMIN_EMAILS) bypass the per-org admin check so they can
// manage any workspace from the People panel.
func (app *application) requireOrgAdminOrInstanceAdmin(w http.ResponseWriter, r *http.Request, orgID, action string) (sessionUser, bool) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return sessionUser{}, false
	}
	if !requireSession(user, w) {
		return sessionUser{}, false
	}
	if user.IsAdmin {
		return user, true
	}
	role, ok := orgRole(user, orgID)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
		return sessionUser{}, false
	}
	if role != roleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only admins can " + action})
		return sessionUser{}, false
	}
	return user, true
}

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
	ID              string         `json:"id"`
	Email           string         `json:"email"`
	Name            string         `json:"name"`
	Username        string         `json:"username"`
	EmailVerifiedAt *time.Time     `json:"emailVerifiedAt,omitempty"`
	CreatedAt       string         `json:"createdAt"`
	Orgs            []adminUserOrg `json:"orgs"`
}

type adminUserOrg struct {
	OrgID      string `json:"orgId"`
	OrgSlug    string `json:"orgSlug"`
	OrgName    string `json:"orgName"`
	MemberID   string `json:"memberId"`
	Role       string `json:"role"`
	IsPersonal bool   `json:"isPersonal"`
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
		       COALESCE(
		           json_agg(json_build_object(
		               'orgId', o.id,
		               'orgSlug', o.slug,
		               'orgName', o.name,
		               'memberId', m.id,
		               'role', m.role,
		               'isPersonal', o.is_personal
		           ) ORDER BY o.is_personal DESC, o.created_at ASC)
		           FILTER (WHERE m.id IS NOT NULL),
		           '[]'::json
		       ) AS orgs
		FROM users u
		LEFT JOIN org_members m ON m.user_id = u.id
		LEFT JOIN organizations o ON o.id = m.org_id
		GROUP BY u.id, u.email, u.name, u.username, u.email_verified_at, u.created_at
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
		var orgsJSON []byte
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Username, &u.EmailVerifiedAt, &createdAt, &orgsJSON); err != nil {
			log.Printf("admin scan user: %v", err)
			continue
		}
		if err := json.Unmarshal(orgsJSON, &u.Orgs); err != nil {
			log.Printf("admin unmarshal user orgs: %v", err)
			u.Orgs = nil
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

	actor, ok := app.requireAdmin(w, r)
	if !ok {
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

	app.logAdminAction(r.Context(), actor, auditActionDeleteUser, auditTargetUser, userID, nil)

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
		s.LiveURL = app.buildLiveURL(s.OrgSlug, s.Slug)
		sites = append(sites, s)
	}

	writeJSON(w, http.StatusOK, map[string]any{"sites": sites})
}

func (app *application) adminSiteByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	actor, ok := app.requireAdmin(w, r)
	if !ok {
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

	app.logAdminAction(r.Context(), actor, auditActionDeleteSite, auditTargetSite, siteID, nil)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
