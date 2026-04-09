package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type claimRequest struct {
	Slug       string `json:"slug"`
	ClaimToken string `json:"claimToken"`
	Name       string `json:"name"`
	Org        string `json:"org,omitempty"`
}

type claimResponse struct {
	ProjectID string `json:"projectId"`
	LiveURL   string `json:"liveUrl"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
}

func (app *application) claimHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var payload claimRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if payload.Slug == "" || payload.ClaimToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug and claimToken are required"})
		return
	}

	orgID, orgSlug, err := resolveOrgFromParam(user, payload.Org)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	ctx := r.Context()
	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		log.Printf("begin tx (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	defer tx.Rollback(ctx)

	var (
		anonID         string
		oldDeployID    string
		oldProjectID   string
		claimTokenHash string
		expiresAt      time.Time
		claimedAt      *time.Time
		storagePrefix  string
		sizeBytes      int64
		fileCount      int
	)
	err = tx.QueryRow(ctx, `
		select ad.id, ad.deploy_id, ad.project_id, ad.claim_token_hash,
		       ad.expires_at, ad.claimed_at, d.storage_prefix, d.size_bytes, d.file_count
		from anonymous_deploys ad
		join deploys d on d.id = ad.deploy_id
		where ad.slug = $1
	`, payload.Slug).Scan(
		&anonID, &oldDeployID, &oldProjectID, &claimTokenHash,
		&expiresAt, &claimedAt, &storagePrefix, &sizeBytes, &fileCount,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "anonymous deploy not found"})
		return
	}
	if claimedAt != nil {
		writeJSON(w, http.StatusGone, map[string]string{"error": "already claimed"})
		return
	}
	if time.Now().After(expiresAt) {
		writeJSON(w, http.StatusGone, map[string]string{"error": "deploy has expired"})
		return
	}

	if hashToken(payload.ClaimToken) != claimTokenHash {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid claim token"})
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = payload.Slug
	}

	newProjectSlug, err := uniqueSlug(ctx, tx, orgID, slugify(name))
	if err != nil {
		log.Printf("unique slug (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	now := time.Now().UTC()
	newProjectID := generateID("proj")

	if _, err := tx.Exec(ctx, `
		insert into projects (id, org_id, slug, name, is_public, created_at, updated_at)
		values ($1, $2, $3, $4, true, $5, $5)
	`, newProjectID, orgID, newProjectSlug, name, now); err != nil {
		log.Printf("insert project (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	newDeployID := generateID("dep")
	if _, err := tx.Exec(ctx, `
		insert into deploys (id, project_id, status, size_bytes, file_count, storage_prefix, created_at)
		values ($1, $2, 'validated', $3, $4, $5, $6)
	`, newDeployID, newProjectID, sizeBytes, fileCount, storagePrefix, now); err != nil {
		log.Printf("insert deploy (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if _, err := tx.Exec(ctx, `
		update projects set current_deploy_id = $2, updated_at = $3 where id = $1
	`, newProjectID, newDeployID, now); err != nil {
		log.Printf("set current deploy (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Repoint FKs to new project+deploy so old records can be deleted
	if _, err := tx.Exec(ctx, `
		update anonymous_deploys
		set claimed_at = $4, deploy_id = $2, project_id = $3
		where id = $1
	`, anonID, newDeployID, newProjectID, now); err != nil {
		log.Printf("mark claimed (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Hard-delete old project + deploy (FK-safe order)
	if _, err := tx.Exec(ctx, `
		update projects set current_deploy_id = null where id = $1
	`, oldProjectID); err != nil {
		log.Printf("null old current_deploy (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if _, err := tx.Exec(ctx, `delete from deploys where id = $1`, oldDeployID); err != nil {
		log.Printf("delete old deploy (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if _, err := tx.Exec(ctx, `delete from projects where id = $1`, oldProjectID); err != nil {
		log.Printf("delete old project (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("commit (claim): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, claimResponse{
		ProjectID: newProjectID,
		LiveURL:   fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, orgSlug, newProjectSlug),
		Name:      name,
		Slug:      newProjectSlug,
	})
}
