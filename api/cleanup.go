// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"log"
	"time"
)

func (app *application) startCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				app.cleanupExpiredTokens(ctx)
				app.cleanupExpiredReplyTokens(ctx)
				app.cleanupExpiredDeviceCodes(ctx)
				app.cleanupSoftDeletedSites(ctx)
				app.cleanupOldAuditLog(ctx)
				app.deviceTokens.expire(time.Now())
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func (app *application) runCleanupQuery(ctx context.Context, label, sql string) {
	tag, err := app.db.Exec(ctx, sql)
	if err != nil {
		log.Printf("cleanup %s: %v", label, err)
		return
	}
	if n := tag.RowsAffected(); n > 0 {
		log.Printf("cleanup: removed %d %s", n, label)
	}
}

func (app *application) cleanupExpiredTokens(ctx context.Context) {
	app.runCleanupQuery(ctx, "expired email tokens", `delete from email_tokens where expires_at < now()`)
}

func (app *application) cleanupExpiredReplyTokens(ctx context.Context) {
	app.runCleanupQuery(ctx, "expired comment reply tokens", `delete from comment_reply_tokens where expires_at < now()`)
}

func (app *application) cleanupExpiredDeviceCodes(ctx context.Context) {
	app.runCleanupQuery(ctx, "expired device codes", `delete from device_codes where expires_at < now()`)
}

func (app *application) cleanupSoftDeletedSites(ctx context.Context) {
	rows, err := app.db.Query(ctx, `
		SELECT p.id, d.id, d.storage_prefix
		FROM sites p
		LEFT JOIN deploys d ON d.site_id = p.id
		WHERE p.deleted_at IS NOT NULL AND p.deleted_at < now() - interval '7 days'
		LIMIT 100
	`)
	if err != nil {
		log.Printf("cleanup soft-deleted sites query: %v", err)
		return
	}
	defer rows.Close()

	type entry struct {
		siteID        string
		deployID      *string
		storagePrefix *string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.siteID, &e.deployID, &e.storagePrefix); err != nil {
			log.Printf("cleanup soft-deleted sites scan: %v", err)
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
				log.Printf("cleanup soft-deleted site list R2: %v", err)
				continue
			}
			if len(keys) > 0 {
				if err := app.store.deleteObjects(ctx, keys); err != nil {
					log.Printf("cleanup soft-deleted site delete R2: %v", err)
				}
			}
		}
	}

	// Collect unique site IDs
	siteIDs := make(map[string]bool)
	for _, e := range entries {
		siteIDs[e.siteID] = true
	}

	// Hard-delete deploys then sites
	for sid := range siteIDs {
		if _, err := app.db.Exec(ctx, `DELETE FROM deploys WHERE site_id = $1`, sid); err != nil {
			log.Printf("cleanup soft-deleted site deploys: %v", err)
			continue
		}
		if _, err := app.db.Exec(ctx, `DELETE FROM sites WHERE id = $1 AND deleted_at IS NOT NULL`, sid); err != nil {
			log.Printf("cleanup soft-deleted site: %v", err)
		}
	}

	log.Printf("cleaned up %d soft-deleted sites", len(siteIDs))
}

func (app *application) cleanupOldAuditLog(ctx context.Context) {
	app.runCleanupQuery(ctx, "old audit_log rows", `delete from audit_log where created_at < now() - interval '1 year'`)
}
