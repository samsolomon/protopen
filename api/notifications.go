// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type notification struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	ReadAt     *string                 `json:"readAt"`
	CreatedAt  string                  `json:"createdAt"`
	Actor      *authorSummary          `json:"actor"`
	Comment    *notificationCommentRef `json:"comment"`
}

type notificationCommentRef struct {
	ID         string  `json:"id"`
	Body       string  `json:"body"`
	PagePath   string  `json:"pagePath"`
	SiteID     string  `json:"siteId"`
	SiteName   string  `json:"siteName"`
	SiteSlug   string  `json:"siteSlug"`
	OrgSlug    string  `json:"orgSlug"`
	ParentID   *string `json:"parentId"`
	ResolvedAt *string `json:"resolvedAt"`
}

// notificationsHandler: GET /api/notifications (with optional ?unread=1).
func (app *application) notificationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	unreadOnly := r.URL.Query().Get("unread") == "1"
	args := []any{user.ID}
	unreadClause := ""
	if unreadOnly {
		unreadClause = "and n.read_at is null"
	}

	rows, err := app.db.Query(r.Context(), `
		select n.id, n.type, n.read_at, n.created_at,
		       actor.id, actor.name, actor.username,
		       c.id, c.body, c.page_path, c.parent_id, c.resolved_at,
		       s.id, s.name, s.slug,
		       o.slug
		from notifications n
		join users actor on actor.id = n.actor_id
		left join comments c on c.id = n.comment_id
		left join sites s on s.id = c.site_id
		left join organizations o on o.id = s.org_id
		where n.recipient_id = $1 `+unreadClause+`
		order by n.created_at desc
		limit 100
	`, args...)
	if err != nil {
		log.Printf("list notifications: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load notifications"})
		return
	}
	defer rows.Close()

	notifications := []notification{}
	for rows.Next() {
		var (
			n            notification
			readAt       sql.NullTime
			createdAt    time.Time
			actorID      string
			actorName    string
			actorUser    string
			cID          sql.NullString
			cBody        sql.NullString
			cPagePath    sql.NullString
			cParentID    sql.NullString
			cResolvedAt  sql.NullTime
			sID          sql.NullString
			sName        sql.NullString
			sSlug        sql.NullString
			oSlug        sql.NullString
		)
		if err := rows.Scan(&n.ID, &n.Type, &readAt, &createdAt,
			&actorID, &actorName, &actorUser,
			&cID, &cBody, &cPagePath, &cParentID, &cResolvedAt,
			&sID, &sName, &sSlug,
			&oSlug); err != nil {
			log.Printf("scan notification: %v", err)
			continue
		}
		if readAt.Valid {
			s := readAt.Time.UTC().Format(time.RFC3339)
			n.ReadAt = &s
		}
		n.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		n.Actor = &authorSummary{ID: actorID, Name: actorName, Username: actorUser}
		if cID.Valid {
			ref := &notificationCommentRef{
				ID:       cID.String,
				Body:     cBody.String,
				PagePath: cPagePath.String,
				SiteID:   sID.String,
				SiteName: sName.String,
				SiteSlug: sSlug.String,
				OrgSlug:  oSlug.String,
			}
			if cParentID.Valid {
				p := cParentID.String
				ref.ParentID = &p
			}
			if cResolvedAt.Valid {
				t := cResolvedAt.Time.UTC().Format(time.RFC3339)
				ref.ResolvedAt = &t
			}
			n.Comment = ref
		}
		notifications = append(notifications, n)
	}

	// Inline unread count for the page header.
	var unreadCount int
	if err := app.db.QueryRow(r.Context(), `select count(*) from notifications where recipient_id = $1 and read_at is null`, user.ID).Scan(&unreadCount); err != nil {
		log.Printf("count unread: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": notifications,
		"unreadCount":   unreadCount,
	})
}

// notificationByIDHandler: routes for /api/notifications/:id/read and /api/notifications/read-all.
func (app *application) notificationByIDHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/notifications/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid notification id"})
		return
	}

	if path == "read-all" {
		app.markAllNotificationsReadHandler(w, r)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] != "read" {
		http.NotFound(w, r)
		return
	}
	app.markNotificationReadHandler(w, r, parts[0])
}

func (app *application) markNotificationReadHandler(w http.ResponseWriter, r *http.Request, notificationID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	tag, err := app.db.Exec(r.Context(), `
		update notifications set read_at = now() where id = $1 and recipient_id = $2 and read_at is null
	`, notificationID, user.ID)
	if err != nil {
		log.Printf("mark notification read: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update notification"})
		return
	}
	if tag.RowsAffected() == 0 {
		// Either already read or doesn't belong to caller. Check which to return a useful status.
		var exists bool
		err := app.db.QueryRow(r.Context(), `select exists(select 1 from notifications where id = $1 and recipient_id = $2)`, notificationID, user.ID).Scan(&exists)
		if errors.Is(err, pgx.ErrNoRows) || !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "notification not found"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) markAllNotificationsReadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if _, err := app.db.Exec(r.Context(), `update notifications set read_at = now() where recipient_id = $1 and read_at is null`, user.ID); err != nil {
		log.Printf("mark all read: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update notifications"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
