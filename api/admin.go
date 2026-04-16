package main

import (
	"encoding/json"
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
		LEFT JOIN org_members m ON m.user_id = u.id
		LEFT JOIN organizations o ON o.id = m.org_id AND o.is_personal = true
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

	// Delete personal org (cascades to projects, deploys, org_members)
	if _, err := app.db.Exec(r.Context(), `
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

	// Delete user (cascades remaining org_members, sessions, tokens)
	tag, err := app.db.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		log.Printf("admin delete user: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete user"})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
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
