package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func loadUserOrgs(ctx context.Context, db interface{ Query(context.Context, string, ...any) (pgx.Rows, error) }, userID string) ([]orgInfo, error) {
	rows, err := db.Query(ctx, `
		select o.id, o.slug, o.name, o.is_personal, m.role, o.plan
		from org_members m
		join organizations o on o.id = m.org_id
		where m.user_id = $1
		order by o.is_personal desc, o.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []orgInfo
	for rows.Next() {
		var o orgInfo
		if err := rows.Scan(&o.ID, &o.Slug, &o.Name, &o.IsPersonal, &o.Role, &o.Plan); err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

func (app *application) projectOrgID(ctx context.Context, projectID string) (string, error) {
	var orgID string
	err := app.db.QueryRow(ctx, `select org_id from projects where id = $1 and deleted_at is null`, projectID).Scan(&orgID)
	return orgID, err
}

// requireProjectAccess looks up which org owns a project and verifies the user is a member.
// Returns orgID and role, or an error distinguishing "not found" from "not a member".
func (app *application) requireProjectAccess(ctx context.Context, user sessionUser, projectID string) (string, string, error) {
	orgID, err := app.projectOrgID(ctx, projectID)
	if err != nil {
		return "", "", fmt.Errorf("project not found")
	}
	role, ok := orgRole(user, orgID)
	if !ok {
		return "", "", fmt.Errorf("not a member of this organization")
	}
	return orgID, role, nil
}

func orgRole(user sessionUser, orgID string) (string, bool) {
	for _, o := range user.Orgs {
		if o.ID == orgID {
			return o.Role, true
		}
	}
	return "", false
}

func personalOrg(user sessionUser) *orgInfo {
	for i := range user.Orgs {
		if user.Orgs[i].IsPersonal {
			return &user.Orgs[i]
		}
	}
	return nil
}

func resolveOrgFromParam(user sessionUser, orgParam string) (string, string, error) {
	if orgParam == "" {
		org := personalOrg(user)
		if org == nil {
			return "", "", fmt.Errorf("no personal organization")
		}
		return org.ID, org.Slug, nil
	}
	for _, o := range user.Orgs {
		if o.Slug == orgParam {
			return o.ID, o.Slug, nil
		}
	}
	return "", "", fmt.Errorf("organization not found")
}

func (app *application) listProjects(ctx context.Context, orgID string) ([]project, error) {
	rows, err := app.db.Query(ctx, `
		select
			p.id,
			p.name,
			p.slug,
			p.updated_at,
			(select count(*) from deploys where project_id = p.id) as deploy_count,
			o.slug,
			p.is_public,
			cd.git_branch,
			cd.git_commit_hash,
			cd.git_remote_url
		from projects p
		join organizations o on o.id = p.org_id
		left join deploys cd on cd.id = p.current_deploy_id
		where p.org_id = $1 and p.deleted_at is null
		order by p.updated_at desc
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []project
	for rows.Next() {
		var (
			entry       project
			updatedAt   time.Time
			orgSlug     string
			deployCount int64
		)

		if err := rows.Scan(&entry.ID, &entry.Name, &entry.Slug, &updatedAt, &deployCount, &orgSlug, &entry.IsPublic,
			&entry.GitBranch, &entry.GitCommitHash, &entry.GitRemoteURL); err != nil {
			return nil, err
		}

		entry.DeployCount = int(deployCount)
		entry.UpdatedAt = relativeTime(updatedAt)
		entry.LiveURL = fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, orgSlug, entry.Slug)
		projects = append(projects, entry)
	}

	return projects, rows.Err()
}

func (app *application) deleteProject(ctx context.Context, orgID string, projectID string) (bool, error) {
	commandTag, err := app.db.Exec(ctx, `
		update projects
		set deleted_at = now(), current_deploy_id = null, updated_at = now()
		where id = $1
		  and deleted_at is null
		  and org_id = $2
	`, projectID, orgID)
	if err != nil {
		return false, err
	}

	return commandTag.RowsAffected() > 0, nil
}

func (app *application) upsertProjectFromUpload(ctx context.Context, orgID string, orgSlug string, prepared preparedUpload) (project, error) {
	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return project{}, err
	}
	defer tx.Rollback(ctx)

	payload := prepared.request

	// Load plan limits for the org (anonymous orgs use defaults)
	var limits planLimits
	isAuthenticated := orgID != sentinelOrgID
	if isAuthenticated {
		var plan string
		tx.QueryRow(ctx, `SELECT plan FROM organizations WHERE id = $1`, orgID).Scan(&plan)
		limits = getPlanLimits(plan)

		if plan == "team" {
			var seatCount int
			tx.QueryRow(ctx, `SELECT COUNT(*) FROM org_members WHERE org_id = $1`, orgID).Scan(&seatCount)
			if seatCount > 0 {
				limits.MaxStorageBytes *= int64(seatCount)
			}
		}
	}

	matches, err := exactNameMatches(ctx, tx, orgID, strings.TrimSpace(payload.Name))
	if err != nil {
		return project{}, err
	}

	if err := validateProjectMatchCount(matches, payload.Name); err != nil {
		return project{}, err
	}

	totalSize := int64(0)
	for _, file := range payload.Files {
		totalSize += file.Size
	}

	now := time.Now().UTC()
	var entry projectRecord
	if len(matches) == 1 {
		entry = matches[0]
		if _, err := tx.Exec(ctx, `update projects set updated_at = $2 where id = $1`, entry.ID, now); err != nil {
			return project{}, err
		}
	} else {
		if isAuthenticated && limits.MaxProjects > 0 {
			var projectCount int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM projects WHERE org_id = $1 AND deleted_at IS NULL`, orgID).Scan(&projectCount); err != nil {
				return project{}, err
			}
			if projectCount >= limits.MaxProjects {
				return project{}, fmt.Errorf("site limit exceeded (%d sites)", limits.MaxProjects)
			}
		}

		slug, err := uniqueSlug(ctx, tx, orgID, slugify(payload.Name))
		if err != nil {
			return project{}, err
		}

		entry = projectRecord{
			ID:      generateID("proj"),
			OrgID:   orgID,
			Name:    strings.TrimSpace(payload.Name),
			Slug:    slug,
			OrgSlug: orgSlug,
		}

		if _, err := tx.Exec(ctx, `
			insert into projects (id, org_id, slug, name, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $5)
		`, entry.ID, entry.OrgID, entry.Slug, entry.Name, now); err != nil {
			return project{}, err
		}
	}

	if isAuthenticated {
		var currentStorage int64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(d.size_bytes), 0)
			FROM deploys d
			JOIN projects p ON p.id = d.project_id
			WHERE p.org_id = $1 AND p.deleted_at IS NULL
		`, orgID).Scan(&currentStorage); err != nil {
			return project{}, err
		}
		if currentStorage+totalSize > limits.MaxStorageBytes {
			return project{}, fmt.Errorf("storage limit exceeded")
		}
	}

	deployID := generateID("dep")
	storagePrefix := fmt.Sprintf("dev/%s/%s", entry.ID, deployID)
	if prepared.normalizedTo != "" {
		storagePrefix = prepared.normalizedTo
	}
	var label *string
	if payload.Label != "" {
		label = &payload.Label
	}
	gitCommitHash := stringPtr(payload.GitCommitHash)
	gitBranch := stringPtr(payload.GitBranch)
	gitCommitMessage := stringPtr(payload.GitCommitMessage)
	gitAuthor := stringPtr(payload.GitAuthor)
	gitRemoteURL := stringPtr(payload.GitRemoteURL)
	if _, err := tx.Exec(ctx, `
		insert into deploys (id, project_id, status, size_bytes, file_count, storage_prefix, created_at, label,
			git_commit_hash, git_branch, git_commit_message, git_dirty, git_author, git_remote_url)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, deployID, entry.ID, "validated", totalSize, len(payload.Files), storagePrefix, now, label,
		gitCommitHash, gitBranch, gitCommitMessage, payload.GitDirty, gitAuthor, gitRemoteURL); err != nil {
		return project{}, err
	}

	if _, err := tx.Exec(ctx, `
		update projects
		set current_deploy_id = $2, updated_at = $3
		where id = $1
	`, entry.ID, deployID, now); err != nil {
		return project{}, err
	}

	var oldPrefixes []string
	if isAuthenticated && !limits.DeployHistory {
		rows, err := tx.Query(ctx, `SELECT storage_prefix FROM deploys WHERE project_id = $1 AND id != $2`, entry.ID, deployID)
		if err != nil {
			return project{}, err
		}
		for rows.Next() {
			var prefix string
			if err := rows.Scan(&prefix); err != nil {
				rows.Close()
				return project{}, err
			}
			oldPrefixes = append(oldPrefixes, prefix)
		}
		rows.Close()

		if _, err := tx.Exec(ctx, `DELETE FROM deploys WHERE project_id = $1 AND id != $2`, entry.ID, deployID); err != nil {
			return project{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return project{}, err
	}

	// Clean up old deploy files from R2 in background
	if app.store != nil && len(oldPrefixes) > 0 {
		go func() {
			for _, prefix := range oldPrefixes {
				keys, err := app.store.listObjects(context.Background(), prefix+"/")
				if err != nil {
					log.Printf("cleanup old deploy list: %v", err)
					continue
				}
				if len(keys) > 0 {
					if err := app.store.deleteObjects(context.Background(), keys); err != nil {
						log.Printf("cleanup old deploy delete: %v", err)
					}
				}
			}
		}()
	}

	var isPublic bool
	app.db.QueryRow(ctx, `select is_public from projects where id = $1`, entry.ID).Scan(&isPublic)

	return project{
		ID:          entry.ID,
		Name:        entry.Name,
		Slug:        entry.Slug,
		UpdatedAt:   relativeTime(now),
		DeployCount: countForMatch(matches),
		LiveURL:     fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, orgSlug, entry.Slug),
		IsPublic:    isPublic,
	}, nil
}

func exactNameMatches(ctx context.Context, tx pgx.Tx, orgID string, name string) ([]projectRecord, error) {
	rows, err := tx.Query(ctx, `
		select p.id, p.org_id, p.name, p.slug, o.slug, coalesce(count(d.id), 0) as deploy_count
		from projects p
		join organizations o on o.id = p.org_id
		left join deploys d on d.project_id = p.id
		where p.org_id = $1 and p.name = $2 and p.deleted_at is null
		group by p.id, o.slug
		order by p.updated_at desc
	`, orgID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []projectRecord
	for rows.Next() {
		var entry projectRecord
		var deployCount int64
		if err := rows.Scan(&entry.ID, &entry.OrgID, &entry.Name, &entry.Slug, &entry.OrgSlug, &deployCount); err != nil {
			return nil, err
		}
		entry.Deploys = int(deployCount)
		matches = append(matches, entry)
	}

	return matches, rows.Err()
}

func lookupUserByEmail(ctx context.Context, tx pgx.Tx, email string) (string, string, error) {
	var (
		id       string
		username string
	)
	err := tx.QueryRow(ctx, `select id, username from users where email = $1`, strings.ToLower(strings.TrimSpace(email))).Scan(&id, &username)
	if err != nil {
		return "", "", err
	}

	return id, username, nil
}

func uniqueSlug(ctx context.Context, tx pgx.Tx, orgID string, base string) (string, error) {
	if base == "" {
		base = "prototype"
	}

	slug := base
	for attempt := 2; ; attempt++ {
		var exists bool
		if err := tx.QueryRow(ctx, `
			select exists(
				select 1 from projects where org_id = $1 and slug = $2 and deleted_at is null
			)
		`, orgID, slug).Scan(&exists); err != nil {
			return "", err
		}

		if !exists {
			return slug, nil
		}

		slug = base + "-" + strconv.Itoa(attempt)
	}
}
