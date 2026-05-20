// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	auditActionDeleteUser = "delete_user"
	auditTargetUser       = "user"
)

// systemAuditEntry is a single row appended by background jobs that have no
// human actor (e.g. the auto-private sweeper).
type systemAuditEntry struct {
	TargetID string
	Metadata map[string]any
}

// logAdminAction writes a row to audit_log. Best-effort: an insert failure
// is logged but never returned. A transient DB blip on the audit row must
// not block the action the operator just took (e.g. force-deleting a user).
func (app *application) logAdminAction(ctx context.Context, actor sessionUser, action, targetType, targetID string, metadata map[string]any) {
	var metaJSON []byte
	if len(metadata) > 0 {
		var err error
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			log.Printf("audit log marshal metadata: %v", err)
			metaJSON = nil
		}
	}

	_, err := app.db.Exec(ctx, `
		INSERT INTO audit_log (id, actor_user_id, actor_email, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, generateID("audit"), actor.ID, actor.Email, action, targetType, targetID, metaJSON)
	if err != nil {
		log.Printf("audit log insert (action=%s target=%s/%s): %v", action, targetType, targetID, err)
	}
}

// logSystemActionsTx records one audit_log row per entry as a single
// multi-row INSERT inside the caller's transaction. Used by background jobs
// that need the audit row to commit atomically with the action that produced
// it — unlike logAdminAction, errors are returned so the caller can roll back.
// actorUserID is left NULL; actorEmail carries a system identifier like
// "system:auto-private" (see migration 023 which relaxed actor_user_id).
func logSystemActionsTx(ctx context.Context, tx pgx.Tx, actorEmail, action, targetType string, entries []systemAuditEntry) error {
	if len(entries) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString(`INSERT INTO audit_log (id, actor_user_id, actor_email, action, target_type, target_id, metadata) VALUES `)
	args := make([]any, 0, len(entries)*6)
	for i, e := range entries {
		var metaJSON []byte
		if len(e.Metadata) > 0 {
			j, err := json.Marshal(e.Metadata)
			if err != nil {
				return fmt.Errorf("audit marshal metadata for %s: %w", e.TargetID, err)
			}
			metaJSON = j
		}
		if i > 0 {
			b.WriteString(", ")
		}
		base := i*6 + 1
		fmt.Fprintf(&b, "($%d, NULL, $%d, $%d, $%d, $%d, $%d)", base, base+1, base+2, base+3, base+4, base+5)
		args = append(args, generateID("audit"), actorEmail, action, targetType, e.TargetID, metaJSON)
	}
	if _, err := tx.Exec(ctx, b.String(), args...); err != nil {
		return fmt.Errorf("audit insert (action=%s, n=%d): %w", action, len(entries), err)
	}
	return nil
}
