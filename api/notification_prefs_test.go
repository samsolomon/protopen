// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotificationPrefs_DefaultsWhenNoRow(t *testing.T) {
	app := newTestApp(t)
	seedUser(t, app.db, "usr_np", "np@test", "np", "NP")

	req := authedRequest(t, app, "GET", "/api/notification-preferences", "", "usr_np")
	rec := httptest.NewRecorder()
	app.notificationPrefsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var prefs notificationPrefs
	decodeJSON(t, rec, &prefs)
	if !prefs.EmailOnReply || !prefs.EmailOnMention {
		t.Errorf("absent row should default opted-in, got %+v", prefs)
	}
}

func TestNotificationPrefs_PatchRoundTrip(t *testing.T) {
	app := newTestApp(t)
	seedUser(t, app.db, "usr_np", "np@test", "np", "NP")

	req := authedRequest(t, app, "PATCH", "/api/notification-preferences", `{"emailOnReply":false}`, "usr_np")
	rec := httptest.NewRecorder()
	app.notificationPrefsHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var patched notificationPrefs
	decodeJSON(t, rec, &patched)
	if patched.EmailOnReply || !patched.EmailOnMention {
		t.Errorf("PATCH should set only emailOnReply false, got %+v", patched)
	}

	// A fresh GET reflects the persisted change.
	getReq := authedRequest(t, app, "GET", "/api/notification-preferences", "", "usr_np")
	getRec := httptest.NewRecorder()
	app.notificationPrefsHandler(getRec, getReq)
	var got notificationPrefs
	decodeJSON(t, getRec, &got)
	if got.EmailOnReply || !got.EmailOnMention {
		t.Errorf("GET after PATCH = %+v, want emailOnReply false", got)
	}
}
