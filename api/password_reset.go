// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func (app *application) forgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(payload.Email))
	ok := map[string]bool{"ok": true}

	if email == "" {
		writeJSON(w, http.StatusOK, ok)
		return
	}

	var userID string
	err := app.db.QueryRow(r.Context(), `select id from users where email = $1`, email).Scan(&userID)
	if err != nil {
		writeJSON(w, http.StatusOK, ok)
		return
	}

	// Invalidate any existing unused reset tokens for this user
	_, _ = app.db.Exec(r.Context(), `
		update email_tokens set used_at = now()
		where user_id = $1 and type = $2 and used_at is null
	`, userID, tokenTypeReset)

	token, err := createEmailToken(r.Context(), app.db, userID, tokenTypeReset, "1 hour")
	if err != nil {
		log.Printf("create reset token: %v", err)
		writeJSON(w, http.StatusOK, ok)
		return
	}

	resetURL := app.appOrigin + "/?reset-token=" + token
	app.trySendEmail("password reset", email, func(m *emailClient) error {
		return m.sendPasswordReset(email, resetURL)
	})

	writeJSON(w, http.StatusOK, ok)
}

func (app *application) resetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if payload.Token == "" || payload.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token and password are required"})
		return
	}
	if len(payload.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	userID, err := consumeEmailToken(r.Context(), app.db, payload.Token, tokenTypeReset)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired token"})
		return
	}

	passwordHash, err := hashPassword(payload.Password)
	if err != nil {
		log.Printf("reset password hash: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not reset password"})
		return
	}

	// Update password and verify email in one query (reset proves ownership)
	if _, err := app.db.Exec(r.Context(), `
		update users set password_hash = $1, email_verified_at = coalesce(email_verified_at, now())
		where id = $2
	`, passwordHash, userID); err != nil {
		log.Printf("reset password update: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not reset password"})
		return
	}

	_, _ = app.db.Exec(r.Context(), `delete from sessions where user_id = $1`, userID)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
