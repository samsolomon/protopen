package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type notification struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	ActorName   string     `json:"actorName"`
	ProjectName string     `json:"projectName"`
	CommentID   string     `json:"commentId"`
	BodyPreview string     `json:"bodyPreview"`
	LinkURL     string     `json:"linkUrl"`
	ReadAt      *time.Time `json:"readAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func (app *application) notificationsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.listNotificationsHandler(w, r)
	case http.MethodPost:
		app.markNotificationsReadHandler(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) notificationsCountHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var count int
	if err := app.db.QueryRow(r.Context(), `
		select count(*) from notifications where recipient_id = $1 and read_at is null
	`, user.ID).Scan(&count); err != nil {
		log.Printf("count notifications: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not count notifications"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"count": count})
}

func (app *application) listNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	rows, err := app.db.Query(r.Context(), `
		select n.id, n.type, u.name, p.name, c.id,
		       substring(c.body, 1, 100), o.slug, p.slug,
		       n.read_at, n.created_at
		from notifications n
		join comments c on c.id = n.comment_id
		join users u on u.id = n.actor_id
		join projects p on p.id = c.project_id
		join organizations o on o.id = p.org_id
		where n.recipient_id = $1
		order by n.created_at desc
		limit 50
	`, user.ID)
	if err != nil {
		log.Printf("list notifications: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load notifications"})
		return
	}
	defer rows.Close()

	var notifications []notification
	for rows.Next() {
		var n notification
		var orgSlug, projectSlug string
		if err := rows.Scan(&n.ID, &n.Type, &n.ActorName, &n.ProjectName, &n.CommentID,
			&n.BodyPreview, &orgSlug, &projectSlug,
			&n.ReadAt, &n.CreatedAt); err != nil {
			log.Printf("scan notification: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load notifications"})
			return
		}
		n.LinkURL = fmt.Sprintf("%s/~%s/%s/#vlr-comment=%s", app.contentBaseURL, orgSlug, projectSlug, n.CommentID)
		notifications = append(notifications, n)
	}

	writeJSON(w, http.StatusOK, map[string]any{"notifications": notifications})
}

func (app *application) markNotificationsReadHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var payload struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	now := time.Now().UTC()
	if payload.All {
		if _, err := app.db.Exec(r.Context(), `
			update notifications set read_at = $1 where recipient_id = $2 and read_at is null
		`, now, user.ID); err != nil {
			log.Printf("mark all read: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not mark notifications"})
			return
		}
	} else if len(payload.IDs) > 0 {
		if _, err := app.db.Exec(r.Context(), `
			update notifications set read_at = $1
			where recipient_id = $2 and id = any($3) and read_at is null
		`, now, user.ID, payload.IDs); err != nil {
			log.Printf("mark read: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not mark notifications"})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
