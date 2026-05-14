// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

const settingThumbnailsEnabled = "thumbnails_enabled"

func (app *application) getInstanceSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := app.db.QueryRow(ctx, `select value from instance_settings where key = $1`, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (app *application) setInstanceSetting(ctx context.Context, key, value, updatedBy string) error {
	_, err := app.db.Exec(ctx, `
		insert into instance_settings (key, value, updated_at, updated_by)
		values ($1, $2, now(), $3)
		on conflict (key) do update set value = excluded.value, updated_at = now(), updated_by = excluded.updated_by
	`, key, value, updatedBy)
	return err
}

func (app *application) enableThumbnails() {
	app.thumbnailMu.Lock()
	defer app.thumbnailMu.Unlock()

	if app.thumbnailer.Load() != nil {
		return
	}
	if !app.thumbnailEnv.available {
		return
	}
	t := newThumbnailer(context.Background(), app.thumbnailEnv.token, app.thumbnailEnv.chromiumPath)
	app.thumbnailer.Store(t)
	// Catch up any deploys captured during the disabled window.
	go app.runThumbnailBackstop(context.Background())
}

func (app *application) disableThumbnails() {
	app.thumbnailMu.Lock()
	defer app.thumbnailMu.Unlock()

	old := app.thumbnailer.Swap(nil)
	if old != nil {
		old.close()
	}
}

type adminSettingsResponse struct {
	Thumbnails adminThumbnailSettings `json:"thumbnails"`
}

type adminThumbnailSettings struct {
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Reason    string `json:"reason,omitempty"`
}

func (app *application) instanceSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := app.requireAdmin(w, r); !ok {
			return
		}
		writeJSON(w, http.StatusOK, app.currentSettings())
	case http.MethodPatch:
		app.patchInstanceSettings(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) currentSettings() adminSettingsResponse {
	return adminSettingsResponse{
		Thumbnails: adminThumbnailSettings{
			Available: app.thumbnailEnv.available,
			Enabled:   app.thumbnailer.Load() != nil,
			Reason:    app.thumbnailEnv.reason,
		},
	}
}

func (app *application) patchInstanceSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := app.requireAdmin(w, r)
	if !ok {
		return
	}

	var payload struct {
		ThumbnailsEnabled *bool `json:"thumbnailsEnabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if payload.ThumbnailsEnabled != nil {
		want := *payload.ThumbnailsEnabled
		if want && !app.thumbnailEnv.available {
			writeJSON(w, http.StatusConflict, map[string]string{"error": app.thumbnailEnv.reason})
			return
		}
		if err := app.setInstanceSetting(r.Context(), settingThumbnailsEnabled, strconv.FormatBool(want), user.Email); err != nil {
			log.Printf("set instance setting %s: %v", settingThumbnailsEnabled, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save settings"})
			return
		}
		if want {
			app.enableThumbnails()
		} else {
			app.disableThumbnails()
		}
	}

	writeJSON(w, http.StatusOK, app.currentSettings())
}
