package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (app *application) listProjects(ctx context.Context, email string) ([]project, error) {
	rows, err := app.db.Query(ctx, `
		select
			p.id,
			p.name,
			p.slug,
			p.updated_at,
			coalesce(count(d.id), 0) as deploy_count,
			u.username,
			p.is_public
		from projects p
		join users u on u.id = p.user_id
		left join deploys d on d.project_id = p.id
		where u.email = $1 and p.deleted_at is null
		group by p.id, u.username
		order by p.updated_at desc
	`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []project
	for rows.Next() {
		var (
			entry       project
			updatedAt   time.Time
			username    string
			deployCount int64
		)

		if err := rows.Scan(&entry.ID, &entry.Name, &entry.Slug, &updatedAt, &deployCount, &username, &entry.IsPublic); err != nil {
			return nil, err
		}

		entry.DeployCount = int(deployCount)
		entry.UpdatedAt = relativeTime(updatedAt)
		entry.LiveURL = fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, username, entry.Slug)
		projects = append(projects, entry)
	}

	return projects, rows.Err()
}

func (app *application) deleteProject(ctx context.Context, email string, projectID string) (bool, error) {
	commandTag, err := app.db.Exec(ctx, `
		update projects
		set deleted_at = now(), current_deploy_id = null, updated_at = now()
		where id = $1
		  and deleted_at is null
		  and user_id = (select id from users where email = $2)
	`, projectID, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return false, err
	}

	return commandTag.RowsAffected() > 0, nil
}

func (app *application) upsertProjectFromUpload(ctx context.Context, email string, prepared preparedUpload) (project, error) {
	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return project{}, err
	}
	defer tx.Rollback(ctx)

	payload := prepared.request

	userID, username, err := lookupUserByEmail(ctx, tx, email)
	if err != nil {
		return project{}, err
	}

	matches, err := exactNameMatches(ctx, tx, userID, strings.TrimSpace(payload.Name))
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
		slug, err := uniqueSlug(ctx, tx, userID, slugify(payload.Name))
		if err != nil {
			return project{}, err
		}

		entry = projectRecord{
			ID:       generateID("proj"),
			UserID:   userID,
			Name:     strings.TrimSpace(payload.Name),
			Slug:     slug,
			Username: username,
		}

		if _, err := tx.Exec(ctx, `
			insert into projects (id, user_id, slug, name, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $5)
		`, entry.ID, entry.UserID, entry.Slug, entry.Name, now); err != nil {
			return project{}, err
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
	if _, err := tx.Exec(ctx, `
		insert into deploys (id, project_id, status, size_bytes, file_count, storage_prefix, created_at, label)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
	`, deployID, entry.ID, "validated", totalSize, len(payload.Files), storagePrefix, now, label); err != nil {
		return project{}, err
	}

	if _, err := tx.Exec(ctx, `
		update projects
		set current_deploy_id = $2, updated_at = $3
		where id = $1
	`, entry.ID, deployID, now); err != nil {
		return project{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return project{}, err
	}

	return project{
		ID:          entry.ID,
		Name:        entry.Name,
		Slug:        entry.Slug,
		UpdatedAt:   relativeTime(now),
		DeployCount: countForMatch(matches),
		LiveURL:     fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, username, entry.Slug),
		IsPublic:    true,
	}, nil
}

func exactNameMatches(ctx context.Context, tx pgx.Tx, userID string, name string) ([]projectRecord, error) {
	rows, err := tx.Query(ctx, `
		select p.id, p.user_id, p.name, p.slug, u.username, coalesce(count(d.id), 0) as deploy_count
		from projects p
		join users u on u.id = p.user_id
		left join deploys d on d.project_id = p.id
		where p.user_id = $1 and p.name = $2 and p.deleted_at is null
		group by p.id, u.username
		order by p.updated_at desc
	`, userID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []projectRecord
	for rows.Next() {
		var entry projectRecord
		var deployCount int64
		if err := rows.Scan(&entry.ID, &entry.UserID, &entry.Name, &entry.Slug, &entry.Username, &deployCount); err != nil {
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

func uniqueSlug(ctx context.Context, tx pgx.Tx, userID string, base string) (string, error) {
	if base == "" {
		base = "prototype"
	}

	slug := base
	for attempt := 2; ; attempt++ {
		var exists bool
		if err := tx.QueryRow(ctx, `
			select exists(
				select 1 from projects where user_id = $1 and slug = $2 and deleted_at is null
			)
		`, userID, slug).Scan(&exists); err != nil {
			return "", err
		}

		if !exists {
			return slug, nil
		}

		slug = base + "-" + strconv.Itoa(attempt)
	}
}
