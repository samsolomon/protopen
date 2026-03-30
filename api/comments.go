package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type comment struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"projectId"`
	DeployID   string     `json:"deployId"`
	UserID     string     `json:"userId"`
	UserName   string     `json:"userName"`
	PagePath   string     `json:"pagePath"`
	PinX       *float64   `json:"pinX"`
	PinY       *float64   `json:"pinY"`
	Body       string     `json:"body"`
	ParentID   *string    `json:"parentId"`
	ResolvedAt *time.Time `json:"resolvedAt"`
	ResolvedBy *string    `json:"resolvedBy"`
	CreatedAt  time.Time  `json:"createdAt"`
	Replies    []comment  `json:"replies,omitempty"`
}

const commentColumns = `c.id, c.project_id, c.deploy_id, c.user_id, u.name,
	c.page_path, c.pin_x, c.pin_y, c.body, c.parent_id,
	c.resolved_at, c.resolved_by, c.created_at`

func scanComment(row pgx.Row) (comment, error) {
	var c comment
	err := row.Scan(&c.ID, &c.ProjectID, &c.DeployID, &c.UserID, &c.UserName,
		&c.PagePath, &c.PinX, &c.PinY, &c.Body, &c.ParentID,
		&c.ResolvedAt, &c.ResolvedBy, &c.CreatedAt)
	return c, err
}

func (app *application) commentOwnership(ctx context.Context, commentID string) (projectID string, userID string, err error) {
	err = app.db.QueryRow(ctx, `
		select project_id, user_id from comments where id = $1
	`, commentID).Scan(&projectID, &userID)
	return
}

func (app *application) commentsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.listCommentsHandler(w, r)
	case http.MethodPost:
		app.createCommentHandler(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) commentByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/__velori/api/comments/")
	if i := strings.IndexByte(id, '/'); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing comment id"})
		return
	}

	switch r.Method {
	case http.MethodPatch:
		app.updateCommentHandler(w, r, id)
	case http.MethodDelete:
		app.deleteCommentHandler(w, r, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) listCommentsHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	projectID := r.URL.Query().Get("project_id")
	deployID := r.URL.Query().Get("deploy_id")
	pagePath := r.URL.Query().Get("page_path")

	if projectID == "" || deployID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and deploy_id are required"})
		return
	}

	if _, _, err := app.requireProjectAccess(r.Context(), user, projectID); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	includeResolved := r.URL.Query().Get("include_resolved") == "true"

	query := `select ` + commentColumns + `
		from comments c
		join users u on u.id = c.user_id
		where c.project_id = $1 and c.deploy_id = $2 and c.page_path = $3 and c.parent_id is null
	`
	if !includeResolved {
		query += ` and c.resolved_at is null`
	}
	query += ` order by c.created_at asc`

	rows, err := app.db.Query(r.Context(), query, projectID, deployID, pagePath)
	if err != nil {
		log.Printf("list comments: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load comments"})
		return
	}
	defer rows.Close()

	// Use index-based map to avoid stale pointers from slice reallocation.
	var comments []comment
	var rootIDs []string
	indexMap := make(map[string]int)

	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			log.Printf("scan comment: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load comments"})
			return
		}
		indexMap[c.ID] = len(comments)
		rootIDs = append(rootIDs, c.ID)
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("list comments iteration: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load comments"})
		return
	}

	if len(rootIDs) > 0 {
		replyRows, err := app.db.Query(r.Context(), `
			select `+commentColumns+`
			from comments c
			join users u on u.id = c.user_id
			where c.parent_id = any($1)
			order by c.created_at asc
		`, rootIDs)
		if err != nil {
			log.Printf("list replies: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load comments"})
			return
		}
		defer replyRows.Close()

		for replyRows.Next() {
			c, err := scanComment(replyRows)
			if err != nil {
				log.Printf("scan reply: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load comments"})
				return
			}
			if idx, ok := indexMap[*c.ParentID]; ok {
				comments[idx].Replies = append(comments[idx].Replies, c)
			}
		}
		if err := replyRows.Err(); err != nil {
			log.Printf("list replies iteration: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load comments"})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

func (app *application) createCommentHandler(w http.ResponseWriter, r *http.Request) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var payload struct {
		ProjectID string   `json:"projectId"`
		DeployID  string   `json:"deployId"`
		PagePath  string   `json:"pagePath"`
		PinX      *float64 `json:"pinX"`
		PinY      *float64 `json:"pinY"`
		Body      string   `json:"body"`
		ParentID  *string  `json:"parentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	body := strings.TrimSpace(payload.Body)
	if body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body is required"})
		return
	}
	if payload.ProjectID == "" || payload.DeployID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "projectId and deployId are required"})
		return
	}

	if payload.ParentID == nil {
		if payload.PinX == nil || payload.PinY == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pinX and pinY are required for root comments"})
			return
		}
		if *payload.PinX < 0 || *payload.PinX > 100 || *payload.PinY < 0 || *payload.PinY > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pin coordinates must be between 0 and 100"})
			return
		}
	}

	if _, _, err := app.requireProjectAccess(r.Context(), user, payload.ProjectID); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	// Single-level threading: parent must be a root comment (parent_id is null).
	// The FK constraint on parent_id handles existence validation.
	if payload.ParentID != nil {
		var parentParentID *string
		err := app.db.QueryRow(r.Context(), `
			select parent_id from comments where id = $1
		`, *payload.ParentID).Scan(&parentParentID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent comment not found"})
			return
		}
		if parentParentID != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "replies to replies are not allowed"})
			return
		}
	}

	now := time.Now().UTC()
	id := generateID("cmt")

	if _, err := app.db.Exec(r.Context(), `
		insert into comments (id, project_id, deploy_id, user_id, page_path, pin_x, pin_y, body, parent_id, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
	`, id, payload.ProjectID, payload.DeployID, user.ID, payload.PagePath, payload.PinX, payload.PinY, body, payload.ParentID, now); err != nil {
		log.Printf("create comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create comment"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"comment": comment{
			ID:        id,
			ProjectID: payload.ProjectID,
			DeployID:  payload.DeployID,
			UserID:    user.ID,
			UserName:  user.Name,
			PagePath:  payload.PagePath,
			PinX:      payload.PinX,
			PinY:      payload.PinY,
			Body:      body,
			ParentID:  payload.ParentID,
			CreatedAt: now,
		},
	})
}

func (app *application) updateCommentHandler(w http.ResponseWriter, r *http.Request, commentID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	projectID, ownerID, err := app.commentOwnership(r.Context(), commentID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "comment not found"})
		return
	}

	if _, _, err := app.requireProjectAccess(r.Context(), user, projectID); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	var payload struct {
		Body     *string `json:"body"`
		Resolved *bool   `json:"resolved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if payload.Body != nil {
		if ownerID != user.ID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the author can edit a comment"})
			return
		}
		body := strings.TrimSpace(*payload.Body)
		if body == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body cannot be empty"})
			return
		}
		if _, err := app.db.Exec(r.Context(), `
			update comments set body = $1, updated_at = now() where id = $2
		`, body, commentID); err != nil {
			log.Printf("update comment body: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update comment"})
			return
		}
	}

	if payload.Resolved != nil {
		if *payload.Resolved {
			if _, err := app.db.Exec(r.Context(), `
				update comments set resolved_at = now(), resolved_by = $1, updated_at = now() where id = $2
			`, user.ID, commentID); err != nil {
				log.Printf("resolve comment: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not resolve comment"})
				return
			}
		} else {
			if _, err := app.db.Exec(r.Context(), `
				update comments set resolved_at = null, resolved_by = null, updated_at = now() where id = $1
			`, commentID); err != nil {
				log.Printf("unresolve comment: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not unresolve comment"})
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) deleteCommentHandler(w http.ResponseWriter, r *http.Request, commentID string) {
	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	projectID, ownerID, err := app.commentOwnership(r.Context(), commentID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "comment not found"})
		return
	}

	_, role, err := app.requireProjectAccess(r.Context(), user, projectID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	if ownerID != user.ID && role != roleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the author or an admin can delete a comment"})
		return
	}

	commandTag, err := app.db.Exec(r.Context(), `delete from comments where id = $1`, commentID)
	if err != nil {
		log.Printf("delete comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete comment"})
		return
	}
	if commandTag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "comment not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
