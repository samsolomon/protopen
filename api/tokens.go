package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

type apiToken struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type createTokenRequest struct {
	Name string `json:"name"`
}

func (app *application) tokensHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.listTokensHandler(w, r)
	case http.MethodPost:
		app.createTokenHandler(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) tokenByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	tokenID := strings.TrimPrefix(r.URL.Path, "/api/tokens/")
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" || strings.Contains(tokenID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token id"})
		return
	}

	commandTag, err := app.db.Exec(r.Context(), `
		delete from api_tokens where id = $1 and user_id = (select id from users where email = $2)
	`, tokenID, strings.ToLower(strings.TrimSpace(user.Email)))
	if err != nil {
		log.Printf("delete token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete token"})
		return
	}
	if commandTag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) listTokensHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	rows, err := app.db.Query(r.Context(), `
		select t.id, t.name, t.created_at
		from api_tokens t
		join users u on u.id = t.user_id
		where u.email = $1
		order by t.created_at desc
	`, strings.ToLower(strings.TrimSpace(user.Email)))
	if err != nil {
		log.Printf("list tokens: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load tokens"})
		return
	}
	defer rows.Close()

	var tokens []apiToken
	for rows.Next() {
		var entry apiToken
		var createdAt time.Time
		if err := rows.Scan(&entry.ID, &entry.Name, &createdAt); err != nil {
			log.Printf("scan token: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load tokens"})
			return
		}
		entry.CreatedAt = relativeTime(createdAt)
		tokens = append(tokens, entry)
	}

	writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

func (app *application) createTokenHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var payload createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = "default"
	}

	rawToken := "ptk_" + generateToken(32)
	tokenID := generateID("tok")

	_, err = app.db.Exec(r.Context(), `
		insert into api_tokens (id, user_id, token_hash, name, created_at)
		values ($1, (select id from users where email = $2), $3, $4, $5)
	`, tokenID, strings.ToLower(strings.TrimSpace(user.Email)), hashToken(rawToken), name, time.Now().UTC())
	if err != nil {
		log.Printf("create token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create token"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":    tokenID,
		"name":  name,
		"token": rawToken,
	})
}
