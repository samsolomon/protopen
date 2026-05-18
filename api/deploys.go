// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type deployRecord struct {
	ID               string  `json:"id"`
	Status           string  `json:"status"`
	Label            *string `json:"label"`
	SizeBytes        int64   `json:"sizeBytes"`
	FileCount        int     `json:"fileCount"`
	CreatedAt        string  `json:"createdAt"`
	IsCurrent        bool    `json:"isCurrent"`
	GitCommitHash    *string `json:"gitCommitHash,omitempty"`
	GitBranch        *string `json:"gitBranch,omitempty"`
	GitCommitMessage *string `json:"gitCommitMessage,omitempty"`
	GitDirty         *bool   `json:"gitDirty,omitempty"`
	GitAuthor        *string `json:"gitAuthor,omitempty"`
	GitRemoteURL     *string `json:"gitRemoteURL,omitempty"`
}

func (app *application) listDeploysHandler(w http.ResponseWriter, r *http.Request, siteID string) {
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

	rows, err := app.db.Query(r.Context(), `
		select d.id, d.status, d.label, d.size_bytes, d.file_count, d.created_at, (d.id = p.current_deploy_id) as is_current,
			d.git_commit_hash, d.git_branch, d.git_commit_message, d.git_dirty, d.git_author, d.git_remote_url
		from deploys d
		join sites p on p.id = d.site_id
		where d.site_id = $1 and p.org_id = $2 and p.deleted_at is null
		order by d.created_at desc
	`, siteID, orgID)
	if err != nil {
		log.Printf("list deploys: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load deploys"})
		return
	}
	defer rows.Close()

	var deploys []deployRecord
	for rows.Next() {
		var entry deployRecord
		var createdAt time.Time
		if err := rows.Scan(&entry.ID, &entry.Status, &entry.Label, &entry.SizeBytes, &entry.FileCount, &createdAt, &entry.IsCurrent,
			&entry.GitCommitHash, &entry.GitBranch, &entry.GitCommitMessage, &entry.GitDirty, &entry.GitAuthor, &entry.GitRemoteURL); err != nil {
			log.Printf("scan deploy: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load deploys"})
			return
		}
		entry.CreatedAt = relativeTime(createdAt)
		deploys = append(deploys, entry)
	}

	writeJSON(w, http.StatusOK, map[string]any{"deploys": deploys})
}

func (app *application) rollbackHandler(w http.ResponseWriter, r *http.Request, siteID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	orgID, _, err := app.requireSiteMutate(r.Context(), user, siteID)
	if err != nil {
		writeJSON(w, statusForSiteMutateError(err), map[string]string{"error": err.Error()})
		return
	}

	var payload struct {
		DeployID string `json:"deployId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.DeployID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deployId is required"})
		return
	}

	var valid bool
	err = app.db.QueryRow(r.Context(), `
		select exists(
			select 1 from deploys d
			join sites p on p.id = d.site_id
			where p.id = $1 and p.org_id = $2 and d.id = $3 and p.deleted_at is null
		)
	`, siteID, orgID, payload.DeployID).Scan(&valid)
	if err != nil || !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "site or deploy not found"})
		return
	}

	now := time.Now().UTC()
	_, err = app.db.Exec(r.Context(), `
		update sites set current_deploy_id = $2, updated_at = $3 where id = $1
	`, siteID, payload.DeployID, now)
	if err != nil {
		log.Printf("rollback: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not rollback"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "currentDeployId": payload.DeployID})
}
