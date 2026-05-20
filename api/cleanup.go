// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

// autoPrivateSweepBatch caps the number of sites flipped per cleanup tick so a
// single sweep can't overwhelm the audit log on first-enable against an
// instance with many stale public sites.
const autoPrivateSweepBatch = 500

// autoPrivateSystemActor is recorded in audit_log.actor_email for sweeper
// rows. actor_user_id is NULL (migration 023 relaxed the NOT NULL).
const autoPrivateSystemActor = "system:auto-private"

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
				app.revertExpiredPublicSites(ctx)
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

// revertExpiredPublicSites flips up to autoPrivateSweepBatch public sites
// whose made_public_at is older than the admin-configured threshold back to
// private and records audit_log rows (one batched multi-row INSERT). No-op
// when the policy is disabled. updated_at is deliberately not touched so the
// dashboard doesn't report system-driven reverts as recent owner activity.
//
// Concurrency: the SELECT uses FOR UPDATE SKIP LOCKED so a second instance
// running the same sweep won't double-process the same rows. Today the
// deployment is single-instance per BUILD_PLAN, but this keeps the query
// safe-by-construction if that changes.
func (app *application) revertExpiredPublicSites(ctx context.Context) {
	policy := app.currentVisibilityPolicy(ctx)
	if !policy.autoPrivateEnabled {
		return
	}

	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		log.Printf("auto-private sweep: begin tx: %v", err)
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		select id, name from sites
		where is_public = true
		  and deleted_at is null
		  and made_public_at is not null
		  and made_public_at < now() - make_interval(days => $1)
		order by made_public_at asc
		limit $2
		for update skip locked
	`, policy.autoPrivateAfterDays, autoPrivateSweepBatch)
	if err != nil {
		log.Printf("auto-private sweep: select: %v", err)
		return
	}

	type staleSite struct{ id, name string }
	var stale []staleSite
	for rows.Next() {
		var s staleSite
		if err := rows.Scan(&s.id, &s.name); err != nil {
			rows.Close()
			log.Printf("auto-private sweep: scan: %v", err)
			return
		}
		stale = append(stale, s)
	}
	rows.Close()

	if len(stale) == 0 {
		return
	}

	ids := make([]string, len(stale))
	for i, s := range stale {
		ids[i] = s.id
	}

	// Flip is_public + clear made_public_at; do not touch updated_at.
	if _, err := tx.Exec(ctx, `
		update sites set is_public = false, made_public_at = null
		where id = any($1)
	`, ids); err != nil {
		log.Printf("auto-private sweep: update: %v", err)
		return
	}

	entries := make([]systemAuditEntry, len(stale))
	for i, s := range stale {
		entries[i] = systemAuditEntry{
			TargetID: s.id,
			Metadata: map[string]any{"siteName": s.name, "afterDays": policy.autoPrivateAfterDays},
		}
	}
	if err := logSystemActionsTx(ctx, tx, autoPrivateSystemActor, "auto_private_revert", "site", entries); err != nil {
		log.Printf("auto-private sweep: %v", err)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("auto-private sweep: commit: %v", err)
		return
	}
	log.Printf("auto-private sweep: reverted %d site(s) older than %d day(s)", len(stale), policy.autoPrivateAfterDays)
}
