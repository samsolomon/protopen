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

func (app *application) siteOrgID(ctx context.Context, siteID string) (string, error) {
	var orgID string
	err := app.db.QueryRow(ctx, `select org_id from sites where id = $1 and deleted_at is null`, siteID).Scan(&orgID)
	return orgID, err
}

// requireSiteAccess looks up which org owns a site and verifies the user is a member.
// Returns orgID and role, or an error distinguishing "not found" from "not a member".
func (app *application) requireSiteAccess(ctx context.Context, user sessionUser, siteID string) (string, string, error) {
	orgID, err := app.siteOrgID(ctx, siteID)
	if err != nil {
		return "", "", fmt.Errorf("site not found")
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

func (app *application) listSites(ctx context.Context, orgID string) ([]site, error) {
	rows, err := app.db.Query(ctx, `
		select
			p.id,
			p.name,
			p.slug,
			p.updated_at,
			(select count(*) from deploys where site_id = p.id) as deploy_count,
			o.slug,
			p.is_public,
			cd.git_branch,
			cd.git_commit_hash,
			cd.git_remote_url
		from sites p
		join organizations o on o.id = p.org_id
		left join deploys cd on cd.id = p.current_deploy_id
		where p.org_id = $1 and p.deleted_at is null
		order by p.updated_at desc
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []site
	for rows.Next() {
		var (
			entry       site
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
		sites = append(sites, entry)
	}

	return sites, rows.Err()
}

func (app *application) deleteSite(ctx context.Context, orgID string, siteID string) (bool, error) {
	commandTag, err := app.db.Exec(ctx, `
		update sites
		set deleted_at = now(), current_deploy_id = null, updated_at = now()
		where id = $1
		  and deleted_at is null
		  and org_id = $2
	`, siteID, orgID)
	if err != nil {
		return false, err
	}

	return commandTag.RowsAffected() > 0, nil
}

func (app *application) upsertSiteFromUpload(ctx context.Context, orgID string, orgSlug string, prepared preparedUpload) (site, error) {
	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return site{}, err
	}
	defer tx.Rollback(ctx)

	payload := prepared.request

	// Load plan limits for the org (anonymous orgs use defaults)
	var limits planLimits
	isOwnedOrg := orgID != sentinelOrgID
	if isOwnedOrg {
		var plan string
		if err := tx.QueryRow(ctx, `SELECT plan FROM organizations WHERE id = $1`, orgID).Scan(&plan); err != nil {
			return site{}, fmt.Errorf("could not load organization")
		}
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
		return site{}, err
	}

	if err := validateSiteMatchCount(matches, payload.Name); err != nil {
		return site{}, err
	}

	totalSize := int64(0)
	for _, file := range payload.Files {
		totalSize += file.Size
	}

	now := time.Now().UTC()
	var entry siteRecord
	if len(matches) == 1 {
		entry = matches[0]
		if _, err := tx.Exec(ctx, `update sites set updated_at = $2 where id = $1`, entry.ID, now); err != nil {
			return site{}, err
		}
	} else {
		if isOwnedOrg && limits.MaxSites > 0 {
			var siteCount int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM sites WHERE org_id = $1 AND deleted_at IS NULL`, orgID).Scan(&siteCount); err != nil {
				return site{}, err
			}
			if siteCount >= limits.MaxSites {
				return site{}, fmt.Errorf("site limit exceeded (%d sites)", limits.MaxSites)
			}
		}

		slug, err := uniqueSlug(ctx, tx, orgID, slugify(payload.Name))
		if err != nil {
			return site{}, err
		}

		entry = siteRecord{
			ID:      generateID("site"),
			OrgID:   orgID,
			Name:    strings.TrimSpace(payload.Name),
			Slug:    slug,
			OrgSlug: orgSlug,
		}

		if _, err := tx.Exec(ctx, `
			insert into sites (id, org_id, slug, name, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $5)
		`, entry.ID, entry.OrgID, entry.Slug, entry.Name, now); err != nil {
			return site{}, err
		}
	}

	if isOwnedOrg {
		var currentStorage int64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(d.size_bytes), 0)
			FROM deploys d
			JOIN sites p ON p.id = d.site_id
			WHERE p.org_id = $1 AND p.deleted_at IS NULL
		`, orgID).Scan(&currentStorage); err != nil {
			return site{}, err
		}
		if currentStorage+totalSize > limits.MaxStorageBytes {
			return site{}, fmt.Errorf("storage limit exceeded")
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
		insert into deploys (id, site_id, status, size_bytes, file_count, storage_prefix, created_at, label,
			git_commit_hash, git_branch, git_commit_message, git_dirty, git_author, git_remote_url)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, deployID, entry.ID, "validated", totalSize, len(payload.Files), storagePrefix, now, label,
		gitCommitHash, gitBranch, gitCommitMessage, payload.GitDirty, gitAuthor, gitRemoteURL); err != nil {
		return site{}, err
	}

	if _, err := tx.Exec(ctx, `
		update sites
		set current_deploy_id = $2, updated_at = $3
		where id = $1
	`, entry.ID, deployID, now); err != nil {
		return site{}, err
	}

	var oldPrefixes []string
	if isOwnedOrg && !limits.DeployHistory {
		rows, err := tx.Query(ctx, `SELECT storage_prefix FROM deploys WHERE site_id = $1 AND id != $2`, entry.ID, deployID)
		if err != nil {
			return site{}, err
		}
		for rows.Next() {
			var prefix string
			if err := rows.Scan(&prefix); err != nil {
				rows.Close()
				return site{}, err
			}
			oldPrefixes = append(oldPrefixes, prefix)
		}
		rows.Close()

		if _, err := tx.Exec(ctx, `DELETE FROM deploys WHERE site_id = $1 AND id != $2`, entry.ID, deployID); err != nil {
			return site{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return site{}, err
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
	app.db.QueryRow(ctx, `select is_public from sites where id = $1`, entry.ID).Scan(&isPublic)

	return site{
		ID:          entry.ID,
		Name:        entry.Name,
		Slug:        entry.Slug,
		UpdatedAt:   relativeTime(now),
		DeployCount: countForMatch(matches),
		LiveURL:     fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, orgSlug, entry.Slug),
		IsPublic:    isPublic,
	}, nil
}

func exactNameMatches(ctx context.Context, tx pgx.Tx, orgID string, name string) ([]siteRecord, error) {
	rows, err := tx.Query(ctx, `
		select p.id, p.org_id, p.name, p.slug, o.slug, coalesce(count(d.id), 0) as deploy_count
		from sites p
		join organizations o on o.id = p.org_id
		left join deploys d on d.site_id = p.id
		where p.org_id = $1 and p.name = $2 and p.deleted_at is null
		group by p.id, o.slug
		order by p.updated_at desc
	`, orgID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []siteRecord
	for rows.Next() {
		var entry siteRecord
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
				select 1 from sites where org_id = $1 and slug = $2 and deleted_at is null
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
