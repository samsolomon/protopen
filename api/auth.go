package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (app *application) sessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (app *application) signInHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload authRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid auth payload"})
		return
	}

	user, err := app.authenticateUser(r.Context(), payload.Email, payload.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	if err := app.createSession(w, r.Context(), user.ID); err != nil {
		log.Printf("create session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (app *application) signUpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload authRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid auth payload"})
		return
	}

	user, err := app.registerUser(r.Context(), payload)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	if err := app.createSession(w, r.Context(), user.ID); err != nil {
		log.Printf("create session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (app *application) signOutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(sessionCookie)
	if err == nil && strings.TrimSpace(cookie.Value) != "" {
		hashed := hashToken(cookie.Value)
		_, _ = app.db.Exec(r.Context(), `delete from sessions where token_hash = $1`, hashed)
	}

	app.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) requireSessionUser(r *http.Request) (sessionUser, error) {
	if user, err := app.authenticateBearer(r); err == nil {
		user.IsAdmin = app.isAdmin(user.Email)
		return user, nil
	}

	cookie, err := r.Cookie(sessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return sessionUser{}, fmt.Errorf("missing session")
	}

	hashed := hashToken(cookie.Value)
	var user sessionUser
	err = app.db.QueryRow(r.Context(), `
		select u.id, u.email, u.name, u.username, u.email_verified_at
		from sessions s
		join users u on u.id = s.user_id
		where s.token_hash = $1 and s.expires_at > now()
	`, hashed).Scan(&user.ID, &user.Email, &user.Name, &user.Username, &user.EmailVerifiedAt)
	if err != nil {
		return sessionUser{}, err
	}

	orgs, err := loadUserOrgs(r.Context(), app.db, user.ID)
	if err != nil {
		return sessionUser{}, err
	}
	user.Orgs = orgs
	user.IsAdmin = app.isAdmin(user.Email)

	return user, nil
}

func (app *application) isAdmin(email string) bool {
	for _, e := range app.adminEmails {
		if strings.EqualFold(e, email) {
			return true
		}
	}
	return false
}

func (app *application) authenticateBearer(r *http.Request) (sessionUser, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer vtk_") {
		return sessionUser{}, fmt.Errorf("no bearer token")
	}

	token := strings.TrimPrefix(header, "Bearer ")
	hashed := hashToken(token)
	var user sessionUser
	err := app.db.QueryRow(r.Context(), `
		select u.id, u.email, u.name, u.username, u.email_verified_at
		from api_tokens t
		join users u on u.id = t.user_id
		where t.token_hash = $1
	`, hashed).Scan(&user.ID, &user.Email, &user.Name, &user.Username, &user.EmailVerifiedAt)
	if err != nil {
		return sessionUser{}, err
	}

	orgs, err := loadUserOrgs(r.Context(), app.db, user.ID)
	if err != nil {
		return sessionUser{}, err
	}
	user.Orgs = orgs
	user.isBearerToken = true

	return user, nil
}

func (app *application) authenticateUser(ctx context.Context, email string, password string) (sessionUser, error) {
	var user sessionUser
	var passwordHash string
	err := app.db.QueryRow(ctx, `
		select id, email, name, username, password_hash, email_verified_at
		from users
		where email = $1
	`, strings.ToLower(strings.TrimSpace(email))).Scan(&user.ID, &user.Email, &user.Name, &user.Username, &passwordHash, &user.EmailVerifiedAt)
	if err != nil {
		return sessionUser{}, err
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return sessionUser{}, fmt.Errorf("invalid credentials")
	}

	orgs, err := loadUserOrgs(ctx, app.db, user.ID)
	if err != nil {
		return sessionUser{}, err
	}
	user.Orgs = orgs

	return user, nil
}

func (app *application) registerUser(ctx context.Context, payload authRequest) (sessionUser, error) {
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	password := strings.TrimSpace(payload.Password)
	name := strings.TrimSpace(payload.Name)
	if email == "" || password == "" || name == "" {
		return sessionUser{}, fmt.Errorf("name, email, and password are required")
	}
	if len(password) < 8 {
		return sessionUser{}, fmt.Errorf("password must be at least 8 characters")
	}

	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return sessionUser{}, err
	}
	defer tx.Rollback(ctx)

	username, err := uniqueUsername(ctx, tx, email)
	if err != nil {
		return sessionUser{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return sessionUser{}, err
	}

	now := time.Now().UTC()
	user := sessionUser{ID: generateID("usr"), Email: email, Name: name, Username: username}
	_, err = tx.Exec(ctx, `
		insert into users (id, email, auth_ref, username, name, password_hash, created_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, user.ID, user.Email, "local-password", user.Username, user.Name, string(passwordHash), now)
	if err != nil {
		if isDuplicateKeyError(err) {
			return sessionUser{}, fmt.Errorf("an account with that email already exists")
		}
		return sessionUser{}, err
	}

	if _, err := ensurePersonalOrg(ctx, tx, user.ID, user.Username, user.Name); err != nil {
		return sessionUser{}, err
	}

	// Auto-accept any pending org invites for this email
	hadInvites := false
	inviteRows, err := tx.Query(ctx, `
		select org_id, role from org_invites where email = $1 and expires_at > now()
	`, user.Email)
	if err != nil {
		return sessionUser{}, err
	}
	defer inviteRows.Close()
	for inviteRows.Next() {
		hadInvites = true
		var invOrgID, invRole string
		if err := inviteRows.Scan(&invOrgID, &invRole); err != nil {
			return sessionUser{}, err
		}
		if _, err := tx.Exec(ctx, `
			insert into org_members (id, org_id, user_id, role, created_at)
			values ($1, $2, $3, $4, $5)
		`, generateID("mem"), invOrgID, user.ID, invRole, now); err != nil {
			return sessionUser{}, err
		}
	}
	if _, err := tx.Exec(ctx, `delete from org_invites where email = $1`, user.Email); err != nil {
		return sessionUser{}, err
	}

	// Invited users are already verified (the invite email proves ownership)
	if hadInvites {
		if _, err := tx.Exec(ctx, `update users set email_verified_at = $2 where id = $1`, user.ID, now); err != nil {
			return sessionUser{}, err
		}
		user.EmailVerifiedAt = &now
	}

	if err := tx.Commit(ctx); err != nil {
		return sessionUser{}, err
	}

	// Organic signups get a verification email
	if !hadInvites {
		app.sendVerificationEmail(ctx, user.ID, user.Email, user.Name)
	}

	orgs, err := loadUserOrgs(ctx, app.db, user.ID)
	if err != nil {
		return sessionUser{}, err
	}
	user.Orgs = orgs

	return user, nil
}

func (app *application) createSession(w http.ResponseWriter, ctx context.Context, userID string) error {
	token := generateToken(32)
	_, err := app.db.Exec(ctx, `
		insert into sessions (id, user_id, token_hash, created_at, expires_at)
		values ($1, $2, $3, $4, $5)
	`, generateID("ses"), userID, hashToken(token), time.Now().UTC(), time.Now().UTC().Add(30*24*time.Hour))
	if err != nil {
		return err
	}

	http.SetCookie(w, app.newSessionCookie(token, 30*24*60*60))
	return nil
}

func (app *application) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, app.newSessionCookie("", -1))
}

func (app *application) newSessionCookie(value string, maxAge int) *http.Cookie {
	cookie := &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
	if app.appOrigin != "" && strings.HasPrefix(app.appOrigin, "https://") {
		cookie.Secure = true
	}
	if app.cookieDomain != "" {
		cookie.Domain = app.cookieDomain
	}
	return cookie
}

func (app *application) sendVerificationEmail(ctx context.Context, userID string, email string, name string) {
	token, err := createEmailToken(ctx, app.db, userID, tokenTypeVerify, "24 hours")
	if err != nil {
		log.Printf("create verification token: %v", err)
		return
	}

	verifyURL := app.appOrigin + "/?verify-token=" + token
	app.trySendEmail("verify email", email, func() error {
		return app.mailer.sendVerifyEmail(email, name, verifyURL)
	})
}

func (app *application) verifyEmailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	userID, err := consumeEmailToken(r.Context(), app.db, payload.Token, tokenTypeVerify)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired token"})
		return
	}

	if _, err := app.db.Exec(r.Context(), `update users set email_verified_at = now() where id = $1 and email_verified_at is null`, userID); err != nil {
		log.Printf("verify email: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not verify email"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) resendVerificationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if user.EmailVerifiedAt != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	_, _ = app.db.Exec(r.Context(), `
		update email_tokens set used_at = now()
		where user_id = $1 and type = $2 and used_at is null
	`, user.ID, tokenTypeVerify)

	app.sendVerificationEmail(r.Context(), user.ID, user.Email, user.Name)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func uniqueUsername(ctx context.Context, tx pgx.Tx, email string) (string, error) {
	base := slugify(strings.Split(email, "@")[0])
	if base == "" {
		base = "user"
	}
	username := base
	for attempt := 2; ; attempt++ {
		var exists bool
		if err := tx.QueryRow(ctx, `
			select exists(
				select 1 from users where username = $1
				union all
				select 1 from organizations where slug = $1 and is_personal = false
			)
		`, username).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return username, nil
		}
		username = base + strconv.Itoa(attempt)
	}
}
