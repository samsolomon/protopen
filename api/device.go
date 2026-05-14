// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

func (app *application) deviceCodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	code := generateToken(16) // 32 hex chars (128 bits)
	id := generateID("dvc")
	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute)

	if _, err := app.db.Exec(r.Context(), `
		insert into device_codes (id, code, status, created_at, expires_at)
		values ($1, $2, 'pending', $3, $4)
	`, id, code, now, expiresAt); err != nil {
		log.Printf("create device code: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create device code"})
		return
	}

	verifyURL := app.appOrigin + "/auth/device?code=" + code
	writeJSON(w, http.StatusCreated, map[string]any{
		"deviceCode": code,
		"verifyUrl":  verifyURL,
		"expiresIn":  600,
	})
}

func (app *application) deviceCodePollHandler(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/api/auth/device/")
	code = strings.TrimRight(code, "/")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		app.pollDeviceCode(w, r, code)
	case http.MethodPost:
		app.approveDeviceCode(w, r, code)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) pollDeviceCode(w http.ResponseWriter, r *http.Request, code string) {
	var status string
	err := app.db.QueryRow(r.Context(), `
		select status from device_codes
		where code = $1 and expires_at > now()
	`, code).Scan(&status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device code not found or expired"})
		return
	}

	if status == "pending" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
		return
	}

	if status == "complete" {
		// claim is atomic and single-use: a concurrent poll gets the empty
		// string. The DB status flip to 'consumed' is best-effort accounting.
		token := app.deviceTokens.claim(code, time.Now())
		if token == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "device code not found or expired"})
			return
		}
		if _, err := app.db.Exec(r.Context(), `update device_codes set status = 'consumed' where code = $1`, code); err != nil {
			log.Printf("mark device code consumed: %v", err)
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "complete", "token": token})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "device code not found or expired"})
}

func (app *application) approveDeviceCode(w http.ResponseWriter, r *http.Request, code string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	tx, err := app.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	defer tx.Rollback(r.Context())

	// Verify and lock the device code
	var id string
	err = tx.QueryRow(r.Context(), `
		select id from device_codes
		where code = $1 and status = 'pending' and expires_at > now()
	`, code).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device code not found or expired"})
		return
	}

	// Create an API token
	rawToken := "ptk_" + generateToken(32)
	tokenID := generateID("tok")
	now := time.Now().UTC()

	if _, err := tx.Exec(r.Context(), `
		insert into api_tokens (id, user_id, token_hash, name, created_at)
		values ($1, $2, $3, $4, $5)
	`, tokenID, user.ID, hashToken(rawToken), "cli", now); err != nil {
		log.Printf("create token for device code: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create token"})
		return
	}

	// Update device code — WHERE status = 'pending' prevents concurrent approvals.
	// The raw token stays out of the DB; it lives in app.deviceTokens until
	// the CLI polls and claims it.
	var expiresAt time.Time
	row := tx.QueryRow(r.Context(), `
		update device_codes set user_id = $1, status = 'complete'
		where id = $2 and status = 'pending'
		returning expires_at
	`, user.ID, id)
	if err := row.Scan(&expiresAt); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "device code already approved"})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	app.deviceTokens.put(code, rawToken, expiresAt)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
