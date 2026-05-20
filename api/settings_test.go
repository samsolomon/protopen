// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// adminApp returns a test app with one admin user and that user's session.
func adminApp(t *testing.T) (*application, string) {
	t.Helper()
	app := newTestApp(t)
	app.adminEmails = []string{"admin@test"}
	seedUser(t, app.db, "usr_admin", "admin@test", "admin", "Admin")
	return app, "usr_admin"
}

func TestAdminSettings_NeverReturnsSecrets(t *testing.T) {
	app, admin := adminApp(t)
	mustExec(t, app.db, `insert into instance_settings (key, value) values
		('email_provider', 'resend'),
		('email_resend_key', 'rk_supersecret_value')`)

	req := authedRequest(t, app, "GET", "/api/admin/settings", "", admin)
	rec := httptest.NewRecorder()
	app.instanceSettingsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "rk_supersecret_value") {
		t.Fatalf("response leaked the secret value:\n%s", rec.Body.String())
	}
	var resp adminSettingsResponse
	decodeJSON(t, rec, &resp)
	if !resp.Email.ResendKeySet {
		t.Error("resendKeySet should be true when a key is stored")
	}
}

func TestAdminSettings_BlankSecretKeepsStoredValue(t *testing.T) {
	app, admin := adminApp(t)
	mustExec(t, app.db, `insert into instance_settings (key, value) values
		('email_provider', 'resend'),
		('email_resend_key', 'rk_original')`)

	// A blank resendKey must not clobber the stored secret.
	req := authedRequest(t, app, "PATCH", "/api/admin/settings",
		`{"email":{"provider":"resend","resendKey":""}}`, admin)
	rec := httptest.NewRecorder()
	app.instanceSettingsHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var stored string
	if err := app.db.QueryRow(context.Background(),
		`select value from instance_settings where key = 'email_resend_key'`).Scan(&stored); err != nil {
		t.Fatalf("read stored key: %v", err)
	}
	if stored != "rk_original" {
		t.Errorf("blank secret clobbered the stored value: got %q", stored)
	}

	// A non-empty value does replace it.
	req2 := authedRequest(t, app, "PATCH", "/api/admin/settings",
		`{"email":{"resendKey":"rk_rotated"}}`, admin)
	rec2 := httptest.NewRecorder()
	app.instanceSettingsHandler(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("rotate PATCH expected 200, got %d", rec2.Code)
	}
	if err := app.db.QueryRow(context.Background(),
		`select value from instance_settings where key = 'email_resend_key'`).Scan(&stored); err != nil {
		t.Fatalf("read rotated key: %v", err)
	}
	if stored != "rk_rotated" {
		t.Errorf("non-empty secret should replace the value: got %q", stored)
	}
}

func TestAdminSettings_NonAdminForbidden(t *testing.T) {
	app, _ := adminApp(t)
	seedUser(t, app.db, "usr_plain", "plain@test", "plain", "Plain")

	req := authedRequest(t, app, "GET", "/api/admin/settings", "", "usr_plain")
	rec := httptest.NewRecorder()
	app.instanceSettingsHandler(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", rec.Code)
	}
}
