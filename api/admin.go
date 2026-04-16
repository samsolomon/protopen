package main

import (
	"encoding/json"
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
	Plan            *string    `json:"plan,omitempty"`
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
		       o.id, o.plan
		FROM users u
		LEFT JOIN LATERAL (
			SELECT o.id, o.plan FROM org_members m
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
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Username, &u.EmailVerifiedAt, &createdAt, &u.OrgID, &u.Plan); err != nil {
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

func (app *application) adminOrgByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, ok := app.requireAdmin(w, r); !ok {
		return
	}

	orgID := strings.TrimPrefix(r.URL.Path, "/api/admin/orgs/")
	orgID = strings.TrimRight(orgID, "/")
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "org ID is required"})
		return
	}

	var payload struct {
		Plan string `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if _, ok := plans[payload.Plan]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid plan: must be tinkerer, pro, or team"})
		return
	}

	tag, err := app.db.Exec(r.Context(), `UPDATE organizations SET plan = $1 WHERE id = $2`, payload.Plan, orgID)
	if err != nil {
		log.Printf("admin update org plan: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update plan"})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plan": payload.Plan})
}

type adminProject struct {
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

func (app *application) adminProjectsHandler(w http.ResponseWriter, r *http.Request) {
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
		FROM projects p
		JOIN organizations o ON o.id = p.org_id
		LEFT JOIN deploys d ON d.project_id = p.id
		WHERE p.deleted_at IS NULL
		GROUP BY p.id, o.slug, o.name
		ORDER BY p.updated_at DESC
		LIMIT 200
	`)
	if err != nil {
		log.Printf("admin list projects: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load projects"})
		return
	}
	defer rows.Close()

	var projects []adminProject
	for rows.Next() {
		var p adminProject
		var updatedAt time.Time
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &updatedAt, &p.DeployCount, &p.StorageBytes, &p.OrgSlug, &p.OrgName, &p.IsPublic); err != nil {
			log.Printf("admin scan project: %v", err)
			continue
		}
		p.UpdatedAt = relativeTime(updatedAt)
		p.LiveURL = fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, p.OrgSlug, p.Slug)
		projects = append(projects, p)
	}

	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (app *application) adminProjectByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, ok := app.requireAdmin(w, r); !ok {
		return
	}

	projectID := strings.TrimPrefix(r.URL.Path, "/api/admin/projects/")
	projectID = strings.TrimRight(projectID, "/")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project ID is required"})
		return
	}

	tag, err := app.db.Exec(r.Context(), `
		UPDATE projects SET deleted_at = now(), current_deploy_id = null, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, projectID)
	if err != nil {
		log.Printf("admin delete project: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete project"})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
