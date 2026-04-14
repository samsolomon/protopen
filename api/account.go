package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type updateProfileRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type deleteAccountRequest struct {
	Password string `json:"password"`
}

func (app *application) accountHandler(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/account/")
	switch action {
	case "profile":
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		app.updateProfileHandler(w, r)
	case "password":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		app.changePasswordHandler(w, r)
	case "delete":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		app.deleteAccountHandler(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (app *application) updateProfileHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var payload updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	name := strings.TrimSpace(payload.Name)
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	if name == "" || email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and email are required"})
		return
	}

	var updated sessionUser
	err = app.db.QueryRow(r.Context(), `
		update users set name = $1, email = $2 where id = $3
		returning id, email, name, username
	`, name, email, user.ID).Scan(&updated.ID, &updated.Email, &updated.Name, &updated.Username)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an account with that email already exists"})
			return
		}
		log.Printf("update profile: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update profile"})
		return
	}

	orgs, err := loadUserOrgs(r.Context(), app.db, updated.ID)
	if err != nil {
		log.Printf("load orgs after profile update: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update profile"})
		return
	}
	updated.Orgs = orgs

	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func (app *application) changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var payload changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if len(payload.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	if err := app.verifyUserPassword(r.Context(), user.ID, payload.CurrentPassword); err != nil {
		if errors.Is(err, errIncorrectPassword) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "current password is incorrect"})
			return
		}
		log.Printf("change password verify: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not change password"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(payload.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("change password hash: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not change password"})
		return
	}

	if _, err := app.db.Exec(r.Context(), `update users set password_hash = $1 where id = $2`, string(newHash), user.ID); err != nil {
		log.Printf("change password update: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not change password"})
		return
	}

	// Invalidate all other sessions so a compromised session can't survive a password change
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		currentHash := hashToken(cookie.Value)
		app.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id = $1 AND token_hash != $2`, user.ID, currentHash)
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) deleteAccountHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var payload deleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if err := app.verifyUserPassword(r.Context(), user.ID, payload.Password); err != nil {
		if errors.Is(err, errIncorrectPassword) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "incorrect password"})
			return
		}
		log.Printf("delete account verify: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete account"})
		return
	}

	// Check if sole admin of any non-personal org with other members
	var blockedCount int
	if err := app.db.QueryRow(r.Context(), `
		select count(*) from organizations o
		join org_members m on m.org_id = o.id
		where m.user_id = $1 and m.role = $2 and o.is_personal = false
		and (select count(*) from org_members where org_id = o.id and role = $2) = 1
		and (select count(*) from org_members where org_id = o.id) > 1
	`, user.ID, roleAdmin).Scan(&blockedCount); err != nil {
		log.Printf("delete account org check: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete account"})
		return
	}
	if blockedCount > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "you are the sole admin of an organization with other members — transfer ownership first"})
		return
	}

	// Delete personal org first (cascades to projects), then delete user
	if _, err := app.db.Exec(r.Context(), `
		delete from organizations where id in (
			select o.id from organizations o
			join org_members m on m.org_id = o.id
			where m.user_id = $1 and o.is_personal = true
		)
	`, user.ID); err != nil {
		log.Printf("delete personal org: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete account"})
		return
	}

	if _, err := app.db.Exec(r.Context(), `delete from users where id = $1`, user.ID); err != nil {
		log.Printf("delete account: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete account"})
		return
	}

	app.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

var errIncorrectPassword = fmt.Errorf("incorrect password")

func (app *application) verifyUserPassword(ctx context.Context, userID string, password string) error {
	var hash string
	if err := app.db.QueryRow(ctx, `select password_hash from users where id = $1`, userID).Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return errIncorrectPassword
	}
	return nil
}
