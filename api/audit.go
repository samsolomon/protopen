// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"log"
)

const (
	auditActionDeleteUser = "delete_user"
	auditTargetUser       = "user"
)

// logAdminAction writes a row to audit_log. Best-effort: an insert failure is
// logged but never returned. A transient DB blip on the audit row must not
// block the action the operator just took (e.g. force-deleting a user).
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
