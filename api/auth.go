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

	"github.com/jackc/pgx/v5/pgxpool"
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

	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) requireSessionUser(r *http.Request) (sessionUser, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return sessionUser{}, fmt.Errorf("missing session")
	}

	hashed := hashToken(cookie.Value)
	var user sessionUser
	err = app.db.QueryRow(r.Context(), `
		select u.id, u.email, u.name, u.username
		from sessions s
		join users u on u.id = s.user_id
		where s.token_hash = $1 and s.expires_at > now()
	`, hashed).Scan(&user.ID, &user.Email, &user.Name, &user.Username)
	if err != nil {
		return sessionUser{}, err
	}

	return user, nil
}

func (app *application) authenticateUser(ctx context.Context, email string, password string) (sessionUser, error) {
	var user sessionUser
	var passwordHash string
	err := app.db.QueryRow(ctx, `
		select id, email, name, username, password_hash
		from users
		where email = $1
	`, strings.ToLower(strings.TrimSpace(email))).Scan(&user.ID, &user.Email, &user.Name, &user.Username, &passwordHash)
	if err != nil {
		return sessionUser{}, err
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return sessionUser{}, fmt.Errorf("invalid credentials")
	}

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

	username, err := uniqueUsername(ctx, app.db, email)
	if err != nil {
		return sessionUser{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return sessionUser{}, err
	}

	user := sessionUser{ID: generateID("usr"), Email: email, Name: name, Username: username}
	_, err = app.db.Exec(ctx, `
		insert into users (id, email, auth_ref, username, name, password_hash, created_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, user.ID, user.Email, "local-password", user.Username, user.Name, string(passwordHash), time.Now().UTC())
	if err != nil {
		if strings.Contains(err.Error(), "users_email_key") || strings.Contains(err.Error(), "duplicate") {
			return sessionUser{}, fmt.Errorf("an account with that email already exists")
		}
		return sessionUser{}, err
	}

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

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   30 * 24 * 60 * 60,
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func uniqueUsername(ctx context.Context, db *pgxpool.Pool, email string) (string, error) {
	base := slugify(strings.Split(email, "@")[0])
	if base == "" {
		base = "user"
	}
	username := base
	for attempt := 2; ; attempt++ {
		var exists bool
		if err := db.QueryRow(ctx, `select exists(select 1 from users where username = $1)`, username).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return username, nil
		}
		username = base + strconv.Itoa(attempt)
	}
}
