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
