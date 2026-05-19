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
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)


type comment struct {
	ID              string         `json:"id"`
	SiteID          string         `json:"siteId"`
	DeployID        string         `json:"deployId"`
	PagePath        string         `json:"pagePath"`
	PinX            *float64       `json:"pinX"`
	PinY            *float64       `json:"pinY"`
	ElementSelector *string        `json:"elementSelector"`
	ElementOffsetX  *float64       `json:"elementOffsetX"`
	ElementOffsetY  *float64       `json:"elementOffsetY"`
	Body            string         `json:"body"`
	ParentID        *string        `json:"parentId"`
	ResolvedAt      *string        `json:"resolvedAt"`
	ResolvedBy      *string        `json:"resolvedBy"`
	CreatedAt       string         `json:"createdAt"`
	Author          *authorSummary `json:"author"`
	GuestName       *string        `json:"guestName"`
}

type createCommentRequest struct {
	DeployID        string   `json:"deployId,omitempty"`
	PagePath        string   `json:"pagePath"`
	PinX            *float64 `json:"pinX,omitempty"`
	PinY            *float64 `json:"pinY,omitempty"`
	ParentID        *string  `json:"parentId,omitempty"`
	Body            string   `json:"body"`
	ElementSelector string   `json:"elementSelector,omitempty"`
	ElementOffsetX  *float64 `json:"elementOffsetX,omitempty"`
	ElementOffsetY  *float64 `json:"elementOffsetY,omitempty"`
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

	meta, err := app.loadSiteMeta(r.Context(), siteID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site not found"})
		return
	}

	// Guests can read comments on public sites; signed-in users get the same
	// view scoped by org membership. Private sites require membership.
	user, sessErr := app.requireSessionUser(r)
	if sessErr != nil {
		if !meta.IsPublic {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
	} else {
		if _, _, err := app.requireSiteAccess(r.Context(), user, siteID); err != nil {
			if !meta.IsPublic {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
				return
			}
		}
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
		select c.id, c.site_id, c.deploy_id, c.page_path, c.pin_x, c.pin_y,
		       c.element_selector, c.element_offset_x, c.element_offset_y,
		       c.body, c.parent_id, c.resolved_at, c.resolved_by, c.created_at,
		       u.id, u.name, u.username, c.guest_name
		from comments c
		left join users u on u.id = c.user_id
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
			c              comment
			pinX           sql.NullFloat64
			pinY           sql.NullFloat64
			elementSel     sql.NullString
			elementOffsetX sql.NullFloat64
			elementOffsetY sql.NullFloat64
			parentID       sql.NullString
			resolvedAt     sql.NullTime
			resolvedBy     sql.NullString
			createdAt      time.Time
			authorID       sql.NullString
			authorName     sql.NullString
			authorUser     sql.NullString
			guestName      sql.NullString
		)
		if err := rows.Scan(&c.ID, &c.SiteID, &c.DeployID, &c.PagePath, &pinX, &pinY,
			&elementSel, &elementOffsetX, &elementOffsetY,
			&c.Body, &parentID, &resolvedAt, &resolvedBy, &createdAt,
			&authorID, &authorName, &authorUser, &guestName); err != nil {
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
		if elementSel.Valid {
			s := elementSel.String
			c.ElementSelector = &s
		}
		if elementOffsetX.Valid {
			x := elementOffsetX.Float64
			c.ElementOffsetX = &x
		}
		if elementOffsetY.Valid {
			y := elementOffsetY.Float64
			c.ElementOffsetY = &y
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
		if authorID.Valid {
			c.Author = &authorSummary{ID: authorID.String, Name: authorName.String, Username: authorUser.String}
		}
		if guestName.Valid {
			g := guestName.String
			c.GuestName = &g
		}
		comments = append(comments, c)
	}

	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// createSiteCommentHandler: POST /api/sites/:siteID/comments
//
// Requires a signed-in org member. Cross-origin requests must include
// X-Protopen-Client: runtime — the custom header forces a CORS preflight
// that a malicious cross-site form cannot satisfy.
func (app *application) createSiteCommentHandler(w http.ResponseWriter, r *http.Request, siteID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if app.commentLimiter != nil {
		ip := clientIP(r, app.trustedProxyHeader)
		allowed, retryAfter := app.commentLimiter.allow(ip)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded, try again later"})
			return
		}
	}

	if !app.requireRuntimeOrigin(w, r) {
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	orgID, _, err := app.requireSiteAccess(r.Context(), user, siteID)
	if err != nil {
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

	elementSelectorArg := clampSelector(req.ElementSelector)

	if _, err := tx.Exec(r.Context(), `
		insert into comments (
			id, site_id, deploy_id, user_id, page_path,
			pin_x, pin_y, body, parent_id,
			element_selector, element_offset_x, element_offset_y,
			created_at, updated_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
	`, commentID, siteID, deployID, user.ID, req.PagePath,
		req.PinX, req.PinY, body, req.ParentID,
		elementSelectorArg, req.ElementOffsetX, req.ElementOffsetY,
		now); err != nil {
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
	}

	// Fan-out: union of thread subscribers (replies only) and site
	// subscribers (every comment). Mentions override the notification type
	// for the mentioned user and also subscribe them to the thread.
	recipients := map[string]string{}

	var rootArg *string
	if rootID != "" {
		rootArg = &rootID
	}
	subRows, err := tx.Query(r.Context(), `
		select user_id from site_subscriptions where site_id = $1
		union
		select user_id from comment_subscriptions where $2::text is not null and comment_id = $2
	`, siteID, rootArg)
	if err == nil {
		for subRows.Next() {
			var uid string
			if err := subRows.Scan(&uid); err != nil {
				continue
			}
			recipients[uid] = notificationTypeCommentReply
		}
		subRows.Close()
	} else {
		log.Printf("fan-out subs: %v", err)
	}

	if mentions := parseMentions(body); len(mentions) > 0 {
		mentionedIDs, err := resolveMentionUserIDs(r.Context(), tx, orgID, mentions)
		if err != nil {
			log.Printf("resolve mentions: %v", err)
		}
		for _, uid := range mentionedIDs {
			recipients[uid] = notificationTypeCommentMention
			if err := upsertCommentSubscription(r.Context(), tx, subscriptionRoot, uid); err != nil {
				log.Printf("subscribe mentioned: %v", err)
			}
		}
	}

	delete(recipients, user.ID)

	if len(recipients) > 0 {
		var (
			args         []any
			placeholders []string
		)
		i := 1
		for uid, ntype := range recipients {
			placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", i, i+1, i+2, i+3, i+4, i+5))
			args = append(args, generateID("notif"), uid, user.ID, ntype, commentID, now)
			i += 6
		}
		query := "insert into notifications (id, recipient_id, actor_id, type, comment_id, created_at) values " + strings.Join(placeholders, ", ")
		if _, err := tx.Exec(r.Context(), query, args...); err != nil {
			log.Printf("create notifications: %v", err)
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create comment"})
		return
	}

	resp := comment{
		ID:              commentID,
		SiteID:          siteID,
		DeployID:        deployID,
		PagePath:        req.PagePath,
		PinX:            req.PinX,
		PinY:            req.PinY,
		ElementSelector: elementSelectorArg,
		ElementOffsetX:  req.ElementOffsetX,
		ElementOffsetY:  req.ElementOffsetY,
		Body:            body,
		ParentID:        req.ParentID,
		CreatedAt:       now.Format(time.RFC3339),
		Author: &authorSummary{
			ID:       user.ID,
			Name:     user.Name,
			Username: user.Username,
		},
	}
	writeJSON(w, http.StatusCreated, map[string]any{"comment": resp})
}

// commentContextHandler: GET /api/sites/by-slug/:org/:slug/comment-context
//
// Bootstrap endpoint for the injected runtime, which only knows the URL
// slugs. Returns the siteID + current deployID so subsequent comment API
// calls can use the ID-keyed endpoints.
func (app *application) commentContextHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/sites/by-slug/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 3 || parts[2] != "comment-context" {
		http.NotFound(w, r)
		return
	}
	orgSlug, siteSlug := parts[0], parts[1]
	if orgSlug == "" || siteSlug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid slug"})
		return
	}
	meta, deployID, err := app.loadSiteMetaBySlug(r.Context(), orgSlug, siteSlug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site not found"})
		return
	}
	// For private sites, only return context to org members so anonymous
	// visitors can't enumerate site IDs.
	if !meta.IsPublic {
		user, sessErr := app.requireSessionUser(r)
		if sessErr != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if _, _, err := app.requireSiteAccess(r.Context(), user, meta.ID); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"siteId":   meta.ID,
		"deployId": deployID,
		"isPublic": meta.IsPublic,
		"siteName": meta.Name,
	})
}

// mentionCandidatesHandler: GET /api/sites/:siteID/mention-candidates?q=...
//
// Returns up to 10 org members whose username starts with q. Auth required:
// only signed-in org members can mention. Guests get a 401 — the runtime
// should hide the autocomplete UI for them.
func (app *application) mentionCandidatesHandler(w http.ResponseWriter, r *http.Request, siteID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	orgID, _, err := app.requireSiteAccess(r.Context(), user, siteID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	prefix := q + "%"
	rows, err := app.db.Query(r.Context(), `
		select u.id, u.name, u.username
		from users u
		join org_members m on m.user_id = u.id
		where m.org_id = $1 and lower(u.username) like $2
		order by u.username
		limit 10
	`, orgID, prefix)
	if err != nil {
		log.Printf("mention candidates: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load candidates"})
		return
	}
	defer rows.Close()
	out := []authorSummary{}
	for rows.Next() {
		var a authorSummary
		if err := rows.Scan(&a.ID, &a.Name, &a.Username); err != nil {
			continue
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": out})
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
	case http.MethodPatch:
		app.updateCommentHandler(w, r, commentID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type updateCommentRequest struct {
	PinX            *float64 `json:"pinX,omitempty"`
	PinY            *float64 `json:"pinY,omitempty"`
	ElementSelector *string  `json:"elementSelector,omitempty"`
	ElementOffsetX  *float64 `json:"elementOffsetX,omitempty"`
	ElementOffsetY  *float64 `json:"elementOffsetY,omitempty"`
	// ClearAnchor unsets element_selector + element_offset_{x,y} in one
	// shot. The runtime sends this on drag drop so the pin becomes purely
	// x/y-positioned and gets the dashed "unanchored" outline.
	ClearAnchor bool `json:"clearAnchor,omitempty"`
}

// updateCommentHandler: PATCH /api/comments/{commentID}
// Updates pin position / anchor fields. Author or org admin only. Only
// fields present in the request body are mutated; omitted fields keep
// their current values.
func (app *application) updateCommentHandler(w http.ResponseWriter, r *http.Request, commentID string) {
	if !app.requireRuntimeOrigin(w, r) {
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var (
		authorID sql.NullString
		siteID   string
	)
	if err := app.db.QueryRow(r.Context(), `select user_id, site_id from comments where id = $1`, commentID).Scan(&authorID, &siteID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "comment not found"})
			return
		}
		log.Printf("lookup comment for update: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update comment"})
		return
	}

	_, role, err := app.requireSiteAccess(r.Context(), user, siteID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	isAuthor := authorID.Valid && authorID.String == user.ID
	if !isAuthor && role != roleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the comment author or an org admin can update this"})
		return
	}

	var req updateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// nil pointers mean "field omitted" — COALESCE keeps the current value.
	// clearAnchor wins over a populated ElementSelector (only one matters in
	// practice; the runtime never sends both).
	var selectorArg *string
	if req.ClearAnchor {
		selectorArg = nil
	} else if req.ElementSelector != nil {
		selectorArg = clampSelector(*req.ElementSelector)
	}

	if _, err := app.db.Exec(r.Context(), `
		update comments
		set pin_x = coalesce($2, pin_x),
		    pin_y = coalesce($3, pin_y),
		    element_selector = case
		        when $7::boolean then null
		        when $4::text is not null then $4
		        else element_selector
		    end,
		    element_offset_x = case when $7::boolean then null else coalesce($5, element_offset_x) end,
		    element_offset_y = case when $7::boolean then null else coalesce($6, element_offset_y) end,
		    updated_at = now()
		where id = $1
	`, commentID, req.PinX, req.PinY, selectorArg, req.ElementOffsetX, req.ElementOffsetY, req.ClearAnchor); err != nil {
		log.Printf("update comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update comment"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) deleteCommentHandler(w http.ResponseWriter, r *http.Request, commentID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var (
		authorID sql.NullString
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
	isAuthor := authorID.Valid && authorID.String == user.ID
	if !isAuthor && role != roleAdmin {
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
