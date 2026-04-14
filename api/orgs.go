package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (app *application) orgsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.listOrgsHandler(w, r)
	case http.MethodPost:
		app.createOrgHandler(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) orgByIDHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/", 3)
	orgID := parts[0]
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid org id"})
		return
	}

	if len(parts) >= 2 {
		switch parts[1] {
		case "members":
			if len(parts) == 3 && parts[2] != "" {
				memberID := strings.TrimRight(parts[2], "/")
				switch r.Method {
				case http.MethodDelete:
					app.removeOrgMemberHandler(w, r, orgID, memberID)
				case http.MethodPatch:
					app.updateOrgMemberHandler(w, r, orgID, memberID)
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			} else {
				switch r.Method {
				case http.MethodGet:
					app.listOrgMembersHandler(w, r, orgID)
				case http.MethodPost:
					app.addOrgMemberHandler(w, r, orgID)
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}
		default:
			http.NotFound(w, r)
		}
		return
	}

	http.NotFound(w, r)
}

func (app *application) listOrgsHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"orgs": user.Orgs})
}

func (app *application) createOrgHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !requireSession(user, w) {
		return
	}

	var payload struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	name := strings.TrimSpace(payload.Name)
	slug := slugify(strings.TrimSpace(payload.Slug))
	if name == "" || slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and slug are required"})
		return
	}

	var collision bool
	if err := app.db.QueryRow(r.Context(), `
		select exists(
			select 1 from users where username = $1
			union all
			select 1 from organizations where slug = $1
		)
	`, slug).Scan(&collision); err != nil {
		log.Printf("check slug collision: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create organization"})
		return
	}
	if collision {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "that slug is already taken"})
		return
	}

	now := time.Now().UTC()
	orgID := generateID("org")
	if _, err := app.db.Exec(r.Context(), `
		insert into organizations (id, slug, name, is_personal, created_at, updated_at)
		values ($1, $2, $3, false, $4, $4)
	`, orgID, slug, name, now); err != nil {
		log.Printf("create org: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create organization"})
		return
	}

	if _, err := app.db.Exec(r.Context(), `
		insert into org_members (id, org_id, user_id, role, created_at)
		values ($1, $2, $3, $4, $5)
	`, generateID("mem"), orgID, user.ID, roleAdmin, now); err != nil {
		log.Printf("create org membership: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create organization"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"org": orgInfo{ID: orgID, Slug: slug, Name: name, IsPersonal: false, Role: roleAdmin},
	})
}

type orgMember struct {
	ID       string `json:"id"`
	UserID   string `json:"userId"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (app *application) listOrgMembersHandler(w http.ResponseWriter, r *http.Request, orgID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if _, ok := orgRole(user, orgID); !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
		return
	}

	rows, err := app.db.Query(r.Context(), `
		select m.id, u.id, u.name, u.email, u.username, m.role
		from org_members m
		join users u on u.id = m.user_id
		where m.org_id = $1
		order by m.created_at
	`, orgID)
	if err != nil {
		log.Printf("list org members: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load members"})
		return
	}
	defer rows.Close()

	var members []orgMember
	for rows.Next() {
		var m orgMember
		if err := rows.Scan(&m.ID, &m.UserID, &m.Name, &m.Email, &m.Username, &m.Role); err != nil {
			log.Printf("scan org member: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load members"})
			return
		}
		members = append(members, m)
	}

	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (app *application) addOrgMemberHandler(w http.ResponseWriter, r *http.Request, orgID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !requireSession(user, w) {
		return
	}

	role, ok := orgRole(user, orgID)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
		return
	}
	if role != roleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only admins can add members"})
		return
	}

	var payload struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(payload.Email))
	memberRole := payload.Role
	if memberRole == "" {
		memberRole = roleMember
	}
	if memberRole != roleAdmin && memberRole != roleMember {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be admin or member"})
		return
	}
	if email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}

	var targetUserID string
	err = app.db.QueryRow(r.Context(), `select id from users where email = $1`, email).Scan(&targetUserID)
	if err == nil {
		now := time.Now().UTC()
		_, err = app.db.Exec(r.Context(), `
			insert into org_members (id, org_id, user_id, role, created_at)
			values ($1, $2, $3, $4, $5)
		`, generateID("mem"), orgID, targetUserID, memberRole, now)
		if err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "user is already a member"})
				return
			}
			log.Printf("add org member: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not add member"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "status": "added"})
		return
	}

	now := time.Now().UTC()
	_, err = app.db.Exec(r.Context(), `
		insert into org_invites (id, org_id, email, role, invited_by, created_at, expires_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, generateID("inv"), orgID, email, memberRole, user.ID, now, now.Add(7*24*time.Hour))
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an invite for this email already exists"})
			return
		}
		log.Printf("create org invite: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create invite"})
		return
	}

	orgName := "your team"
	for _, org := range user.Orgs {
		if org.ID == orgID {
			orgName = org.Name
			break
		}
	}
	signupURL := app.appOrigin + "/sign-up?email=" + url.QueryEscape(email)
	app.trySendEmail("org invite", email, func() error {
		return app.mailer.sendOrgInvite(email, orgName, user.Name, signupURL)
	})

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "status": "invited"})
}

func (app *application) removeOrgMemberHandler(w http.ResponseWriter, r *http.Request, orgID string, memberID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !requireSession(user, w) {
		return
	}

	role, ok := orgRole(user, orgID)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
		return
	}
	if role != roleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only admins can remove members"})
		return
	}

	// Prevent removing the last admin
	var adminCount int
	if err := app.db.QueryRow(r.Context(), `
		select count(*) from org_members where org_id = $1 and role = $2
	`, orgID, roleAdmin).Scan(&adminCount); err != nil {
		log.Printf("count admins: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not remove member"})
		return
	}

	var memberRoleVal string
	if err := app.db.QueryRow(r.Context(), `
		select role from org_members where id = $1 and org_id = $2
	`, memberID, orgID).Scan(&memberRoleVal); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "member not found"})
		return
	}

	if memberRoleVal == roleAdmin && adminCount <= 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot remove the last admin"})
		return
	}

	commandTag, err := app.db.Exec(r.Context(), `
		delete from org_members where id = $1 and org_id = $2
	`, memberID, orgID)
	if err != nil {
		log.Printf("remove org member: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not remove member"})
		return
	}
	if commandTag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "member not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) updateOrgMemberHandler(w http.ResponseWriter, r *http.Request, orgID string, memberID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !requireSession(user, w) {
		return
	}

	role, ok := orgRole(user, orgID)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
		return
	}
	if role != roleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only admins can change roles"})
		return
	}

	var payload struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if payload.Role != roleAdmin && payload.Role != roleMember {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be admin or member"})
		return
	}

	var targetUserID string
	var currentRole string
	if err := app.db.QueryRow(r.Context(), `
		select user_id, role from org_members where id = $1 and org_id = $2
	`, memberID, orgID).Scan(&targetUserID, &currentRole); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "member not found"})
		return
	}

	if targetUserID == user.ID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot change your own role"})
		return
	}

	if currentRole == payload.Role {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if currentRole == roleAdmin && payload.Role == roleMember {
		var adminCount int
		if err := app.db.QueryRow(r.Context(), `
			select count(*) from org_members where org_id = $1 and role = $2
		`, orgID, roleAdmin).Scan(&adminCount); err != nil {
			log.Printf("count admins: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update role"})
			return
		}
		if adminCount <= 1 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot demote the last admin"})
			return
		}
	}

	if _, err := app.db.Exec(r.Context(), `
		update org_members set role = $1 where id = $2 and org_id = $3
	`, payload.Role, memberID, orgID); err != nil {
		log.Printf("update org member role: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update role"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
