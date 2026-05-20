// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- shared fixture ---------------------------------------------------------

type visibilityFixture struct {
	app    *application
	admin  string
	member string
	orgID  string
}

func seedVisibilityFixture(t *testing.T) visibilityFixture {
	t.Helper()
	app := newTestApp(t)
	app.adminEmails = []string{"admin@test"}
	seedUser(t, app.db, "usr_admin", "admin@test", "admin", "Admin")
	seedUser(t, app.db, "usr_member", "member@test", "member", "Member")
	seedOrg(t, app.db, "org_team", "team", "Team", false)
	seedOrgMember(t, app.db, "org_team", "usr_admin", roleAdmin)
	seedOrgMember(t, app.db, "org_team", "usr_member", roleMember)
	return visibilityFixture{app: app, admin: "usr_admin", member: "usr_member", orgID: "org_team"}
}

// --- settings round-trip ---------------------------------------------------

func TestVisibilitySettings_RoundTrip(t *testing.T) {
	f := seedVisibilityFixture(t)

	// GET before any writes: defaults flow through (defaultSitePrivate=false,
	// autoPrivateEnabled=false, days=30 fallback).
	req := authedRequest(t, f.app, "GET", "/api/admin/settings", "", f.admin)
	rec := httptest.NewRecorder()
	f.app.instanceSettingsHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET 1: %d %s", rec.Code, rec.Body.String())
	}
	var resp adminSettingsResponse
	decodeJSON(t, rec, &resp)
	if resp.Visibility.DefaultSitePrivate || resp.Visibility.AutoPrivateEnabled {
		t.Fatalf("unset settings should default to false: %+v", resp.Visibility)
	}
	if resp.Visibility.AutoPrivateAfterDays != defaultAutoPrivateAfterDays {
		t.Fatalf("days default: expected %d got %d", defaultAutoPrivateAfterDays, resp.Visibility.AutoPrivateAfterDays)
	}

	// PATCH all three.
	body := `{"visibility":{"defaultSitePrivate":true,"autoPrivateEnabled":true,"autoPrivateAfterDays":7}}`
	req = authedRequest(t, f.app, "PATCH", "/api/admin/settings", body, f.admin)
	rec = httptest.NewRecorder()
	f.app.instanceSettingsHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: %d %s", rec.Code, rec.Body.String())
	}
	decodeJSON(t, rec, &resp)
	if !resp.Visibility.DefaultSitePrivate || !resp.Visibility.AutoPrivateEnabled || resp.Visibility.AutoPrivateAfterDays != 7 {
		t.Fatalf("PATCH did not persist: %+v", resp.Visibility)
	}
}

func TestVisibilitySettings_DaysValidation(t *testing.T) {
	f := seedVisibilityFixture(t)

	for _, days := range []int{0, -1, maxAutoPrivateAfterDays + 1} {
		body := fmt.Sprintf(`{"visibility":{"autoPrivateAfterDays":%d}}`, days)
		req := authedRequest(t, f.app, "PATCH", "/api/admin/settings", body, f.admin)
		rec := httptest.NewRecorder()
		f.app.instanceSettingsHandler(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("days=%d expected 400 got %d body=%s", days, rec.Code, rec.Body.String())
		}
	}
}

// --- upload default + payload override -------------------------------------

func TestUpsertSiteFromUpload_AppliesInstanceDefault(t *testing.T) {
	f := seedVisibilityFixture(t)

	mustExec(t, f.app.db, `insert into instance_settings (key, value) values ('default_site_private', 'true')`)

	prepared := preparedUpload{request: uploadRequest{
		Name:  "Default Private",
		Files: []fileMeta{{Name: "index.html", Size: 10, Path: "index.html"}},
	}}
	result, err := f.app.upsertSiteFromUpload(context.Background(), f.orgID, "team", f.member, roleMember, prepared)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if result.IsPublic {
		t.Fatalf("expected new site to be private when default_site_private=true")
	}

	var madePublicAt *time.Time
	if err := f.app.db.QueryRow(context.Background(),
		`select made_public_at from sites where id = $1`, result.ID).Scan(&madePublicAt); err != nil {
		t.Fatalf("scan made_public_at: %v", err)
	}
	if madePublicAt != nil {
		t.Fatalf("private site should have null made_public_at, got %v", madePublicAt)
	}
}

func TestUpsertSiteFromUpload_ExplicitFlagOverridesDefault(t *testing.T) {
	f := seedVisibilityFixture(t)
	mustExec(t, f.app.db, `insert into instance_settings (key, value) values ('default_site_private', 'true')`)

	pub := true
	prepared := preparedUpload{request: uploadRequest{
		Name:     "Forced Public",
		Files:    []fileMeta{{Name: "index.html", Size: 10, Path: "index.html"}},
		IsPublic: &pub,
	}}
	result, err := f.app.upsertSiteFromUpload(context.Background(), f.orgID, "team", f.member, roleMember, prepared)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !result.IsPublic {
		t.Fatalf("explicit isPublic=true must override default_site_private=true")
	}
	var madePublicAt *time.Time
	if err := f.app.db.QueryRow(context.Background(),
		`select made_public_at from sites where id = $1`, result.ID).Scan(&madePublicAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if madePublicAt == nil {
		t.Fatalf("public site must have made_public_at set")
	}
}


// --- toggle writes / clears made_public_at ---------------------------------

func TestUpdateSiteHandler_TogglesMadePublicAt(t *testing.T) {
	f := seedVisibilityFixture(t)
	creator := f.member
	mustExec(t, f.app.db, `insert into sites (id, org_id, slug, name, is_public, created_by) values ('site_t1', $1, 'toggle', 'Toggle', false, $2)`, f.orgID, creator)

	req := authedRequest(t, f.app, "PATCH", "/api/sites/site_t1", `{"isPublic":true}`, f.member)
	rec := httptest.NewRecorder()
	f.app.updateSiteHandler(rec, req, "site_t1")
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle to public: %d %s", rec.Code, rec.Body.String())
	}
	var madePublicAt *time.Time
	if err := f.app.db.QueryRow(context.Background(),
		`select made_public_at from sites where id = 'site_t1'`).Scan(&madePublicAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if madePublicAt == nil {
		t.Fatalf("false→true must set made_public_at")
	}
	firstStamp := *madePublicAt

	req = authedRequest(t, f.app, "PATCH", "/api/sites/site_t1", `{"isPublic":false}`, f.member)
	rec = httptest.NewRecorder()
	f.app.updateSiteHandler(rec, req, "site_t1")
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle to private: %d %s", rec.Code, rec.Body.String())
	}
	if err := f.app.db.QueryRow(context.Background(),
		`select made_public_at from sites where id = 'site_t1'`).Scan(&madePublicAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if madePublicAt != nil {
		t.Fatalf("true→false must null made_public_at, got %v", madePublicAt)
	}

	// Re-publish: clock restarts.
	req = authedRequest(t, f.app, "PATCH", "/api/sites/site_t1", `{"isPublic":true}`, f.member)
	rec = httptest.NewRecorder()
	f.app.updateSiteHandler(rec, req, "site_t1")
	if rec.Code != http.StatusOK {
		t.Fatalf("re-publish: %d %s", rec.Code, rec.Body.String())
	}
	if err := f.app.db.QueryRow(context.Background(),
		`select made_public_at from sites where id = 'site_t1'`).Scan(&madePublicAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if madePublicAt == nil || madePublicAt.Before(firstStamp) {
		t.Fatalf("re-publish must reset clock, got %v (first=%v)", madePublicAt, firstStamp)
	}
}

// --- sweeper ---------------------------------------------------------------

func enableAutoPrivate(t *testing.T, app *application, days int) {
	t.Helper()
	mustExec(t, app.db, `insert into instance_settings (key, value) values
		('auto_private_enabled', 'true'),
		('auto_private_after_days', $1)`, fmt.Sprintf("%d", days))
}

func TestRevertExpiredPublicSites_DisabledNoOp(t *testing.T) {
	f := seedVisibilityFixture(t)
	mustExec(t, f.app.db, `insert into sites (id, org_id, slug, name, is_public, made_public_at, created_by)
		values ('site_s', $1, 's', 'S', true, now() - interval '100 days', $2)`, f.orgID, f.member)

	f.app.revertExpiredPublicSites(context.Background())

	var stillPublic bool
	mustScan(t, f.app, `select is_public from sites where id = 'site_s'`, &stillPublic)
	if !stillPublic {
		t.Fatal("disabled sweep must not flip any site")
	}
}

func TestRevertExpiredPublicSites_FlipsStaleLeavesRecent(t *testing.T) {
	f := seedVisibilityFixture(t)
	enableAutoPrivate(t, f.app, 30)

	mustExec(t, f.app.db, `insert into sites (id, org_id, slug, name, is_public, made_public_at, updated_at, created_by) values
		('site_stale',  $1, 'stale',  'Stale',  true, now() - interval '40 days', now() - interval '40 days', $2),
		('site_fresh',  $1, 'fresh',  'Fresh',  true, now() - interval '10 days', now() - interval '10 days', $2),
		('site_private',$1, 'priv',   'Priv',   false, null,                       now() - interval '40 days', $2)`,
		f.orgID, f.member)

	var staleUpdatedBefore time.Time
	mustScan(t, f.app, `select updated_at from sites where id = 'site_stale'`, &staleUpdatedBefore)

	f.app.revertExpiredPublicSites(context.Background())

	var staleNowPublic, freshStillPublic, privateStillPrivate bool
	var staleUpdatedAfter time.Time
	var stalePublicAt *time.Time
	mustScan(t, f.app, `select is_public from sites where id = 'site_stale'`, &staleNowPublic)
	mustScan(t, f.app, `select is_public from sites where id = 'site_fresh'`, &freshStillPublic)
	mustScan(t, f.app, `select not is_public from sites where id = 'site_private'`, &privateStillPrivate)
	mustScan(t, f.app, `select updated_at from sites where id = 'site_stale'`, &staleUpdatedAfter)
	mustScan(t, f.app, `select made_public_at from sites where id = 'site_stale'`, &stalePublicAt)

	if staleNowPublic {
		t.Fatal("stale site must have been flipped to private")
	}
	if !freshStillPublic {
		t.Fatal("fresh site must remain public")
	}
	if !privateStillPrivate {
		t.Fatal("already-private site must not change")
	}
	if !staleUpdatedAfter.Equal(staleUpdatedBefore) {
		t.Fatalf("sweeper bumped updated_at: before=%v after=%v", staleUpdatedBefore, staleUpdatedAfter)
	}
	if stalePublicAt != nil {
		t.Fatalf("made_public_at must be cleared on revert, got %v", stalePublicAt)
	}

	var auditCount int
	mustScan(t, f.app, `select count(*) from audit_log where action = 'auto_private_revert' and target_id = 'site_stale'`, &auditCount)
	if auditCount != 1 {
		t.Fatalf("expected 1 audit row for stale revert, got %d", auditCount)
	}
}

// mustScan is a single-column QueryRow helper.
func mustScan(t *testing.T, app *application, sql string, dest any) {
	t.Helper()
	if err := app.db.QueryRow(context.Background(), sql).Scan(dest); err != nil {
		t.Fatalf("scan %q: %v", sql, err)
	}
}
