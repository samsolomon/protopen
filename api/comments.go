// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const notificationTypeCommentReply = "comment_reply"

type comment struct {
	ID         string         `json:"id"`
	SiteID     string         `json:"siteId"`
	DeployID   string         `json:"deployId"`
	PagePath   string         `json:"pagePath"`
	PinX       *float64       `json:"pinX"`
	PinY       *float64       `json:"pinY"`
	Body       string         `json:"body"`
	ParentID   *string        `json:"parentId"`
	ResolvedAt *string        `json:"resolvedAt"`
	ResolvedBy *string        `json:"resolvedBy"`
	CreatedAt  string         `json:"createdAt"`
	Author     *authorSummary `json:"author"`
}

type createCommentRequest struct {
	DeployID string   `json:"deployId,omitempty"`
	PagePath string   `json:"pagePath"`
	PinX     *float64 `json:"pinX,omitempty"`
	PinY     *float64 `json:"pinY,omitempty"`
	ParentID *string  `json:"parentId,omitempty"`
	Body     string   `json:"body"`
}

// listSiteCommentsHandler: GET /api/sites/:siteID/comments
// Query params:
//
//	?deployId=...   defaults to the site's current_deploy_id
//	?status=open|resolved|all (default open)
//	?pagePath=...   optional exact-match filter
func (app *application) listSiteCommentsHandler(w http.ResponseWriter, r *http.Request, siteID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if _, _, err := app.requireSiteAccess(r.Context(), user, siteID); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	q := r.URL.Query()
	deployID := strings.TrimSpace(q.Get("deployId"))
	if deployID == "" {
		var current *string
		if err := app.db.QueryRow(r.Context(), `select current_deploy_id from sites where id = $1`, siteID).Scan(&current); err != nil || current == nil {
			writeJSON(w, http.StatusOK, map[string]any{"comments": []comment{}})
			return
		}
		deployID = *current
	}

	status := q.Get("status")
	if status == "" {
		status = "open"
	}
	var statusClause string
	switch status {
	case "open":
		statusClause = "and c.resolved_at is null"
	case "resolved":
		statusClause = "and c.resolved_at is not null"
	case "all":
		statusClause = ""
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}

	args := []any{siteID, deployID}
	pagePathClause := ""
	if pagePath := q.Get("pagePath"); pagePath != "" {
		pagePathClause = "and c.page_path = $3"
		args = append(args, pagePath)
	}

	query := fmt.Sprintf(`
		select c.id, c.site_id, c.deploy_id, c.page_path, c.pin_x, c.pin_y, c.body,
		       c.parent_id, c.resolved_at, c.resolved_by, c.created_at,
		       u.id, u.name, u.username
		from comments c
		join users u on u.id = c.user_id
		where c.site_id = $1 and c.deploy_id = $2 %s %s
		order by c.created_at asc
	`, statusClause, pagePathClause)

	rows, err := app.db.Query(r.Context(), query, args...)
	if err != nil {
		log.Printf("list comments: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load comments"})
		return
	}
	defer rows.Close()

	comments := []comment{}
	for rows.Next() {
		var (
			c           comment
			pinX        sql.NullFloat64
			pinY        sql.NullFloat64
			parentID    sql.NullString
			resolvedAt  sql.NullTime
			resolvedBy  sql.NullString
			createdAt   time.Time
			authorID    string
			authorName  string
			authorUser  string
		)
		if err := rows.Scan(&c.ID, &c.SiteID, &c.DeployID, &c.PagePath, &pinX, &pinY, &c.Body,
			&parentID, &resolvedAt, &resolvedBy, &createdAt,
			&authorID, &authorName, &authorUser); err != nil {
			log.Printf("scan comment: %v", err)
			continue
		}
		if pinX.Valid {
			x := pinX.Float64
			c.PinX = &x
		}
		if pinY.Valid {
			y := pinY.Float64
			c.PinY = &y
		}
		if parentID.Valid {
			p := parentID.String
			c.ParentID = &p
		}
		if resolvedAt.Valid {
			t := resolvedAt.Time.UTC().Format(time.RFC3339)
			c.ResolvedAt = &t
		}
		if resolvedBy.Valid {
			s := resolvedBy.String
			c.ResolvedBy = &s
		}
		c.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		c.Author = &authorSummary{ID: authorID, Name: authorName, Username: authorUser}
		comments = append(comments, c)
	}

	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// createSiteCommentHandler: POST /api/sites/:siteID/comments
func (app *application) createSiteCommentHandler(w http.ResponseWriter, r *http.Request, siteID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if _, _, err := app.requireSiteAccess(r.Context(), user, siteID); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	var req createCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body required"})
		return
	}
	if len(body) > 8000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body too long"})
		return
	}

	tx, err := app.db.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		log.Printf("create comment begin tx: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create comment"})
		return
	}
	defer tx.Rollback(r.Context())

	deployID := strings.TrimSpace(req.DeployID)
	if deployID == "" {
		var current *string
		if err := tx.QueryRow(r.Context(), `select current_deploy_id from sites where id = $1`, siteID).Scan(&current); err != nil || current == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "site has no current deploy"})
			return
		}
		deployID = *current
	} else {
		var ownerSiteID string
		if err := tx.QueryRow(r.Context(), `select site_id from deploys where id = $1`, deployID).Scan(&ownerSiteID); err != nil || ownerSiteID != siteID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deploy does not belong to this site"})
			return
		}
	}

	// The UI only exposes single-level threads, so parent_id always points
	// at a root. coalesce(parent_id, id) returns the root id directly.
	rootID := ""
	if req.ParentID != nil && *req.ParentID != "" {
		var parentSite, parentRoot string
		err := tx.QueryRow(r.Context(),
			`select site_id, coalesce(parent_id, id) from comments where id = $1`,
			*req.ParentID,
		).Scan(&parentSite, &parentRoot)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent comment not found"})
			return
		}
		if parentSite != siteID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent comment belongs to a different site"})
			return
		}
		rootID = parentRoot
	}

	commentID := generateID("cm")
	now := time.Now().UTC()

	if _, err := tx.Exec(r.Context(), `
		insert into comments (id, site_id, deploy_id, user_id, page_path, pin_x, pin_y, body, parent_id, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
	`, commentID, siteID, deployID, user.ID, req.PagePath, req.PinX, req.PinY, body, req.ParentID, now); err != nil {
		log.Printf("insert comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create comment"})
		return
	}

	subscriptionRoot := rootID
	if subscriptionRoot == "" {
		subscriptionRoot = commentID
	}

	if err := upsertCommentSubscription(r.Context(), tx, subscriptionRoot, user.ID); err != nil {
		log.Printf("subscribe author: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create comment"})
		return
	}

	// Top-level comments also subscribe the site owner so they hear about
	// activity on their work even if they did not participate in the thread.
	if rootID == "" {
		var siteOwner *string
		if err := tx.QueryRow(r.Context(), `select created_by from sites where id = $1`, siteID).Scan(&siteOwner); err == nil && siteOwner != nil && *siteOwner != user.ID {
			if err := upsertCommentSubscription(r.Context(), tx, subscriptionRoot, *siteOwner); err != nil {
				log.Printf("subscribe site owner: %v", err) // non-fatal: comment is inserted
			}
		}
	}

	if rootID != "" {
		subRows, err := tx.Query(r.Context(), `
			select user_id from comment_subscriptions where comment_id = $1 and user_id != $2
		`, rootID, user.ID)
		if err != nil {
			log.Printf("fan-out query: %v", err)
		} else {
			defer subRows.Close()
			recipients := []string{}
			for subRows.Next() {
				var uid string
				if err := subRows.Scan(&uid); err != nil {
					continue
				}
				recipients = append(recipients, uid)
			}
			for _, uid := range recipients {
				if _, err := tx.Exec(r.Context(), `
					insert into notifications (id, recipient_id, actor_id, type, comment_id, created_at)
					values ($1, $2, $3, $4, $5, $6)
				`, generateID("notif"), uid, user.ID, notificationTypeCommentReply, commentID, now); err != nil {
					log.Printf("create notification: %v", err)
				}
			}
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create comment"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"comment": comment{
			ID:        commentID,
			SiteID:    siteID,
			DeployID:  deployID,
			PagePath:  req.PagePath,
			PinX:      req.PinX,
			PinY:      req.PinY,
			Body:      body,
			ParentID:  req.ParentID,
			CreatedAt: now.Format(time.RFC3339),
			Author: &authorSummary{
				ID:       user.ID,
				Name:     user.Name,
				Username: user.Username,
			},
		},
	})
}

func upsertCommentSubscription(ctx context.Context, tx pgx.Tx, commentID, userID string) error {
	_, err := tx.Exec(ctx, `
		insert into comment_subscriptions (id, comment_id, user_id, created_at)
		values ($1, $2, $3, now())
		on conflict (comment_id, user_id) do nothing
	`, generateID("csub"), commentID, userID)
	return err
}

// commentByIDHandler: routes for /api/comments/:id and /api/comments/:id/resolve.
func (app *application) commentByIDHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/comments/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid comment id"})
		return
	}
	parts := strings.SplitN(path, "/", 2)
	commentID := parts[0]

	if len(parts) == 2 {
		switch parts[1] {
		case "resolve":
			app.resolveCommentHandler(w, r, commentID)
		default:
			http.NotFound(w, r)
		}
		return
	}

	switch r.Method {
	case http.MethodDelete:
		app.deleteCommentHandler(w, r, commentID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) deleteCommentHandler(w http.ResponseWriter, r *http.Request, commentID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var (
		authorID string
		siteID   string
	)
	if err := app.db.QueryRow(r.Context(), `select user_id, site_id from comments where id = $1`, commentID).Scan(&authorID, &siteID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "comment not found"})
			return
		}
		log.Printf("lookup comment for delete: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete comment"})
		return
	}

	_, role, err := app.requireSiteAccess(r.Context(), user, siteID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	if user.ID != authorID && role != roleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the comment author or an org admin can delete this"})
		return
	}

	if _, err := app.db.Exec(r.Context(), `delete from comments where id = $1`, commentID); err != nil {
		log.Printf("delete comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete comment"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) resolveCommentHandler(w http.ResponseWriter, r *http.Request, commentID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var siteID string
	var resolvedAt sql.NullTime
	if err := app.db.QueryRow(r.Context(), `select site_id, resolved_at from comments where id = $1`, commentID).Scan(&siteID, &resolvedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "comment not found"})
			return
		}
		log.Printf("lookup comment for resolve: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update comment"})
		return
	}

	if _, _, err := app.requireSiteAccess(r.Context(), user, siteID); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	// Toggle: if already resolved, un-resolve. Otherwise mark resolved by current user.
	if resolvedAt.Valid {
		if _, err := app.db.Exec(r.Context(), `update comments set resolved_at = null, resolved_by = null, updated_at = now() where id = $1`, commentID); err != nil {
			log.Printf("unresolve comment: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update comment"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "resolved": false})
		return
	}
	if _, err := app.db.Exec(r.Context(), `update comments set resolved_at = now(), resolved_by = $2, updated_at = now() where id = $1`, commentID, user.ID); err != nil {
		log.Printf("resolve comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update comment"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "resolved": true})
}
