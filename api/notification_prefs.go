// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5"
)

type notificationPrefs struct {
	EmailOnReply   bool `json:"emailOnReply"`
	EmailOnMention bool `json:"emailOnMention"`
}

// getNotificationPrefs returns a user's email preferences, falling back to the
// opted-in defaults when no row exists.
func (app *application) getNotificationPrefs(ctx context.Context, userID string) (notificationPrefs, error) {
	prefs := notificationPrefs{EmailOnReply: true, EmailOnMention: true}
	err := app.db.QueryRow(ctx,
		`select email_on_reply, email_on_mention from user_notification_prefs where user_id = $1`,
		userID).Scan(&prefs.EmailOnReply, &prefs.EmailOnMention)
	if errors.Is(err, pgx.ErrNoRows) {
		return prefs, nil
	}
	if err != nil {
		return notificationPrefs{}, err
	}
	return prefs, nil
}

// notificationPrefsHandler: GET/PATCH /api/notification-preferences — the
// signed-in user reads or updates their own email notification settings.
func (app *application) notificationPrefsHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !requireSession(user, w) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		prefs, err := app.getNotificationPrefs(r.Context(), user.ID)
		if err != nil {
			log.Printf("get notification prefs: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preferences"})
			return
		}
		writeJSON(w, http.StatusOK, prefs)

	case http.MethodPatch:
		prefs, err := app.getNotificationPrefs(r.Context(), user.ID)
		if err != nil {
			log.Printf("get notification prefs: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preferences"})
			return
		}
		var payload struct {
			EmailOnReply   *bool `json:"emailOnReply"`
			EmailOnMention *bool `json:"emailOnMention"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if payload.EmailOnReply != nil {
			prefs.EmailOnReply = *payload.EmailOnReply
		}
		if payload.EmailOnMention != nil {
			prefs.EmailOnMention = *payload.EmailOnMention
		}
		if _, err := app.db.Exec(r.Context(), `
			insert into user_notification_prefs (user_id, email_on_reply, email_on_mention, updated_at)
			values ($1, $2, $3, now())
			on conflict (user_id) do update set
				email_on_reply = excluded.email_on_reply,
				email_on_mention = excluded.email_on_mention,
				updated_at = now()
		`, user.ID, prefs.EmailOnReply, prefs.EmailOnMention); err != nil {
			log.Printf("save notification prefs: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save preferences"})
			return
		}
		writeJSON(w, http.StatusOK, prefs)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
