package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

func (app *application) startCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				app.cleanupExpiredAnonymousDeploys(ctx)
				app.cleanupExpiredTokens(ctx)
				app.cleanupExpiredDeviceCodes(ctx)
				app.cleanupSoftDeletedProjects(ctx)
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

type expiredDeploy struct {
	anonID        string
	slug          string
	deployID      string
	projectID     string
	storagePrefix string
}

func (app *application) cleanupExpiredAnonymousDeploys(ctx context.Context) {
	rows, err := app.db.Query(ctx, `
		select ad.id, ad.slug, ad.deploy_id, ad.project_id, d.storage_prefix
		from anonymous_deploys ad
		join deploys d on d.id = ad.deploy_id
		where ad.expires_at < now() and ad.claimed_at is null
		limit 100
	`)
	if err != nil {
		log.Printf("cleanup: query expired deploys: %v", err)
		return
	}
	defer rows.Close()

	var expired []expiredDeploy
	for rows.Next() {
		var e expiredDeploy
		if err := rows.Scan(&e.anonID, &e.slug, &e.deployID, &e.projectID, &e.storagePrefix); err != nil {
			log.Printf("cleanup: scan row: %v", err)
			return
		}
		expired = append(expired, e)
	}
	if err := rows.Err(); err != nil {
		log.Printf("cleanup: iterate rows: %v", err)
		return
	}

	if len(expired) == 0 {
		return
	}

	cleaned := 0
	for _, e := range expired {
		if err := app.cleanupOneDeploy(ctx, e); err != nil {
			log.Printf("cleanup: %s: %v", e.slug, err)
			continue
		}
		cleaned++
	}

	log.Printf("cleanup: removed %d expired anonymous deploys", cleaned)
}

func (app *application) cleanupExpiredTokens(ctx context.Context) {
	result, err := app.db.Exec(ctx, `delete from email_tokens where expires_at < now()`)
	if err != nil {
		log.Printf("cleanup: expired tokens: %v", err)
		return
	}
	if n := result.RowsAffected(); n > 0 {
		log.Printf("cleanup: removed %d expired email tokens", n)
	}
}

func (app *application) cleanupOneDeploy(ctx context.Context, e expiredDeploy) error {
	// Delete R2 objects first — if this fails, DB records survive for retry next cycle
	if app.store != nil {
		keys, err := app.store.listObjects(ctx, e.storagePrefix+"/")
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := app.store.deleteObjects(ctx, keys); err != nil {
				return err
			}
		}
	}

	// Hard-delete DB records in FK-safe order
	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `update projects set current_deploy_id = null where id = $1`, e.projectID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from anonymous_deploys where id = $1`, e.anonID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from deploys where id = $1`, e.deployID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from projects where id = $1`, e.projectID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (app *application) cleanupExpiredDeviceCodes(ctx context.Context) {
	tag, err := app.db.Exec(ctx, `delete from device_codes where expires_at < now()`)
	if err != nil {
		log.Printf("cleanup device codes: %v", err)
		return
	}
	if tag.RowsAffected() > 0 {
		log.Printf("cleaned up %d expired device codes", tag.RowsAffected())
	}
}

func (app *application) cleanupSoftDeletedProjects(ctx context.Context) {
	rows, err := app.db.Query(ctx, `
		SELECT p.id, d.id, d.storage_prefix
		FROM projects p
		LEFT JOIN deploys d ON d.project_id = p.id
		WHERE p.deleted_at IS NOT NULL AND p.deleted_at < now() - interval '7 days'
		LIMIT 100
	`)
	if err != nil {
		log.Printf("cleanup soft-deleted projects query: %v", err)
		return
	}
	defer rows.Close()

	type entry struct {
		projectID     string
		deployID      *string
		storagePrefix *string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.projectID, &e.deployID, &e.storagePrefix); err != nil {
			log.Printf("cleanup soft-deleted projects scan: %v", err)
			continue
		}
		entries = append(entries, e)
	}

	if len(entries) == 0 {
		return
	}

	// Delete R2 files for each deploy
	if app.store != nil {
		for _, e := range entries {
			if e.storagePrefix == nil {
				continue
			}
			keys, err := app.store.listObjects(ctx, *e.storagePrefix+"/")
			if err != nil {
				log.Printf("cleanup soft-deleted project list R2: %v", err)
				continue
			}
			if len(keys) > 0 {
				if err := app.store.deleteObjects(ctx, keys); err != nil {
					log.Printf("cleanup soft-deleted project delete R2: %v", err)
				}
			}
		}
	}

	// Collect unique project IDs
	projectIDs := make(map[string]bool)
	for _, e := range entries {
		projectIDs[e.projectID] = true
	}

	// Hard-delete deploys then projects
	for pid := range projectIDs {
		if _, err := app.db.Exec(ctx, `DELETE FROM deploys WHERE project_id = $1`, pid); err != nil {
			log.Printf("cleanup soft-deleted project deploys: %v", err)
			continue
		}
		if _, err := app.db.Exec(ctx, `DELETE FROM projects WHERE id = $1 AND deleted_at IS NOT NULL`, pid); err != nil {
			log.Printf("cleanup soft-deleted project: %v", err)
		}
	}

	log.Printf("cleaned up %d soft-deleted projects", len(projectIDs))
}
