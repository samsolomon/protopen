// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func seedSession(t *testing.T, app *application, userID string) string {
	t.Helper()
	id := generateID("sess")
	token := generateID("tok") + "_" + userID
	hashed := hashToken(token)
	mustExec(t, app.db, `
		insert into sessions (id, user_id, token_hash, expires_at)
		values ($1, $2, $3, now() + interval '1 hour')
	`, id, userID, hashed)
	return token
}

func seedDeploy(t *testing.T, app *application, siteID, deployID string) {
	t.Helper()
	mustExec(t, app.db, `
		insert into deploys (id, site_id, status, storage_prefix, size_bytes, file_count)
		values ($1, $2, 'seeded', $3, 0, 0)
	`, deployID, siteID, "/tmp/seed/"+deployID)
	mustExec(t, app.db, `update sites set current_deploy_id = $2 where id = $1`, siteID, deployID)
}

// commentsFixture stands up a team org with admin + member + outsider users,
// a single site owned by the member, and a current deploy. Returns the IDs
// the tests need.
type commentsFixture struct {
	app      *application
	orgID    string
	siteID   string
	deployID string
	admin    string
	owner    string
	other    string
	outsider string
}

func seedCommentsFixture(t *testing.T) commentsFixture {
	t.Helper()
	app := newTestApp(t)
	orgID := "org_team"
	siteID := "site_1"
	deployID := "dep_1"

	seedUser(t, app.db, "usr_admin", "admin@example.test", "admin", "Admin")
	seedUser(t, app.db, "usr_owner", "owner@example.test", "owner", "Owner")
	seedUser(t, app.db, "usr_other", "other@example.test", "other", "Other")
	seedUser(t, app.db, "usr_outsider", "outsider@example.test", "outsider", "Outsider")
	seedOrg(t, app.db, orgID, "team", "Team", false)
	seedOrgMember(t, app.db, orgID, "usr_admin", roleAdmin)
	seedOrgMember(t, app.db, orgID, "usr_owner", roleMember)
	seedOrgMember(t, app.db, orgID, "usr_other", roleMember)
	creator := "usr_owner"
	seedSite(t, app.db, siteID, orgID, "site-one", "Site One", &creator)
	seedDeploy(t, app, siteID, deployID)

	// Outsider has their own personal org with no overlap.
	seedOrg(t, app.db, "org_outsider", "outsider", "Outsider", true)
	seedOrgMember(t, app.db, "org_outsider", "usr_outsider", roleAdmin)

	return commentsFixture{
		app: app, orgID: orgID, siteID: siteID, deployID: deployID,
		admin: "usr_admin", owner: "usr_owner", other: "usr_other", outsider: "usr_outsider",
	}
}

func authedRequest(t *testing.T, app *application, method, path, body, asUserID string) *http.Request {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != "" {
		bodyReader = bytes.NewReader([]byte(body))
	}
	var req *http.Request
	if bodyReader != nil {
		req = httptest.NewRequest(method, path, bodyReader)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if asUserID != "" {
		token := seedSession(t, app, asUserID)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	}
	return req
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
}

func TestCreateComment_AnyMemberCanPost(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","pinX":0.5,"pinY":0.5,"body":"hello"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct{ Comment comment }
	decodeJSON(t, rec, &out)
	if out.Comment.ID == "" || out.Comment.Body != "hello" {
		t.Fatalf("unexpected comment payload: %+v", out)
	}

	// Author auto-subscribes.
	var subs int
	if err := f.app.db.QueryRow(context.Background(),
		`select count(*) from comment_subscriptions where comment_id = $1 and user_id = $2`,
		out.Comment.ID, f.other).Scan(&subs); err != nil || subs != 1 {
		t.Fatalf("expected author subscription, got count=%d err=%v", subs, err)
	}

	// Site owner auto-subscribes (different user).
	if err := f.app.db.QueryRow(context.Background(),
		`select count(*) from comment_subscriptions where comment_id = $1 and user_id = $2`,
		out.Comment.ID, f.owner).Scan(&subs); err != nil || subs != 1 {
		t.Fatalf("expected site-owner subscription, got count=%d err=%v", subs, err)
	}
}

func TestCreateComment_OutsiderForbidden(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","body":"nope"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.outsider)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateComment_OwnerPostingNoSelfDuplicateSubscription(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","body":"first"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.owner)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct{ Comment comment }
	decodeJSON(t, rec, &out)

	var subs int
	if err := f.app.db.QueryRow(context.Background(),
		`select count(*) from comment_subscriptions where comment_id = $1`,
		out.Comment.ID).Scan(&subs); err != nil || subs != 1 {
		t.Fatalf("expected 1 subscription (author only), got count=%d", subs)
	}
}

func TestCreateComment_ReplyFanOutToRootSubscribers(t *testing.T) {
	f := seedCommentsFixture(t)
	ctx := context.Background()

	// other posts a top-level. Author + owner subscribed.
	body := `{"pagePath":"/","body":"top"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("top-level: %d %s", rec.Code, rec.Body.String())
	}
	var top struct{ Comment comment }
	decodeJSON(t, rec, &top)

	// admin replies. Notifications should fan out to other (author) and owner (site owner),
	// excluding admin.
	replyBody := `{"pagePath":"/","parentId":"` + top.Comment.ID + `","body":"reply"}`
	req = authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", replyBody, f.admin)
	rec = httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reply: %d %s", rec.Code, rec.Body.String())
	}
	var reply struct{ Comment comment }
	decodeJSON(t, rec, &reply)

	var recipients []string
	rows, err := f.app.db.Query(ctx, `select recipient_id from notifications where comment_id = $1 order by recipient_id`, reply.Comment.ID)
	if err != nil {
		t.Fatalf("query notifications: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		_ = rows.Scan(&uid)
		recipients = append(recipients, uid)
	}
	if !equalUnorderedStrings(recipients, []string{f.other, f.owner}) {
		t.Fatalf("expected fan-out to {owner, other}, got %v", recipients)
	}
}

func TestCreateComment_ReplyParentMustMatchSite(t *testing.T) {
	f := seedCommentsFixture(t)

	// Set up a second site in the same org with a comment.
	seedSite(t, f.app.db, "site_2", f.orgID, "site-two", "Site Two", &f.owner)
	seedDeploy(t, f.app, "site_2", "dep_2")
	body := `{"pagePath":"/","body":"top"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/site_2/comments", body, f.owner)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, "site_2")
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed top: %d %s", rec.Code, rec.Body.String())
	}
	var top struct{ Comment comment }
	decodeJSON(t, rec, &top)

	// Try to reply to site_2's comment from site_1.
	replyBody := `{"pagePath":"/","parentId":"` + top.Comment.ID + `","body":"cross-site"}`
	req = authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", replyBody, f.owner)
	rec = httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 cross-site, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteComment_AuthorAllowed(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","body":"mine"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	var out struct{ Comment comment }
	decodeJSON(t, rec, &out)

	req = authedRequest(t, f.app, "DELETE", "/api/comments/"+out.Comment.ID, "", f.other)
	rec = httptest.NewRecorder()
	f.app.commentByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("author delete: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteComment_AdminAllowed(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","body":"others"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	var out struct{ Comment comment }
	decodeJSON(t, rec, &out)

	req = authedRequest(t, f.app, "DELETE", "/api/comments/"+out.Comment.ID, "", f.admin)
	rec = httptest.NewRecorder()
	f.app.commentByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin delete: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteComment_NonAuthorMemberForbidden(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","body":"others"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	var out struct{ Comment comment }
	decodeJSON(t, rec, &out)

	req = authedRequest(t, f.app, "DELETE", "/api/comments/"+out.Comment.ID, "", f.owner)
	rec = httptest.NewRecorder()
	f.app.commentByIDHandler(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-author non-admin: expected 403, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteComment_CascadesReplies(t *testing.T) {
	f := seedCommentsFixture(t)

	// Top from other.
	body := `{"pagePath":"/","body":"top"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	var top struct{ Comment comment }
	decodeJSON(t, rec, &top)

	// Reply from admin.
	replyBody := `{"pagePath":"/","parentId":"` + top.Comment.ID + `","body":"reply"}`
	req = authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", replyBody, f.admin)
	rec = httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reply: %d", rec.Code)
	}

	// Author deletes top. Reply should be gone via CASCADE.
	req = authedRequest(t, f.app, "DELETE", "/api/comments/"+top.Comment.ID, "", f.other)
	rec = httptest.NewRecorder()
	f.app.commentByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete top: %d", rec.Code)
	}

	var n int
	if err := f.app.db.QueryRow(context.Background(),
		`select count(*) from comments where site_id = $1`, f.siteID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("expected 0 comments after cascade, got %d err=%v", n, err)
	}
}

func TestResolveComment_TogglesAndAnyMember(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","body":"to resolve"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	var out struct{ Comment comment }
	decodeJSON(t, rec, &out)

	// Non-author member resolves (allowed).
	req = authedRequest(t, f.app, "POST", "/api/comments/"+out.Comment.ID+"/resolve", "", f.owner)
	rec = httptest.NewRecorder()
	f.app.commentByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve: %d %s", rec.Code, rec.Body.String())
	}

	var resolvedAt *time.Time
	var resolvedBy *string
	if err := f.app.db.QueryRow(context.Background(),
		`select resolved_at, resolved_by from comments where id = $1`, out.Comment.ID).Scan(&resolvedAt, &resolvedBy); err != nil {
		t.Fatalf("read comment: %v", err)
	}
	if resolvedAt == nil || resolvedBy == nil || *resolvedBy != f.owner {
		t.Fatalf("expected resolved by owner, got resolvedAt=%v resolvedBy=%v", resolvedAt, resolvedBy)
	}

	// Toggle off.
	req = authedRequest(t, f.app, "POST", "/api/comments/"+out.Comment.ID+"/resolve", "", f.admin)
	rec = httptest.NewRecorder()
	f.app.commentByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unresolve: %d", rec.Code)
	}
	if err := f.app.db.QueryRow(context.Background(),
		`select resolved_at from comments where id = $1`, out.Comment.ID).Scan(&resolvedAt); err != nil {
		t.Fatalf("read after unresolve: %v", err)
	}
	if resolvedAt != nil {
		t.Fatalf("expected resolved_at null, got %v", resolvedAt)
	}
}

func TestListComments_StatusFilters(t *testing.T) {
	f := seedCommentsFixture(t)

	for i, status := range []string{"open", "resolved"} {
		body := `{"pagePath":"/","body":"c` + status + `"}`
		req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
		rec := httptest.NewRecorder()
		f.app.createSiteCommentHandler(rec, req, f.siteID)
		var out struct{ Comment comment }
		decodeJSON(t, rec, &out)
		if i == 1 {
			req = authedRequest(t, f.app, "POST", "/api/comments/"+out.Comment.ID+"/resolve", "", f.other)
			rec = httptest.NewRecorder()
			f.app.commentByIDHandler(rec, req)
		}
	}

	for _, tc := range []struct {
		status string
		expect int
	}{{"open", 1}, {"resolved", 1}, {"all", 2}} {
		req := authedRequest(t, f.app, "GET", "/api/sites/"+f.siteID+"/comments?status="+tc.status, "", f.owner)
		rec := httptest.NewRecorder()
		f.app.listSiteCommentsHandler(rec, req, f.siteID)
		if rec.Code != http.StatusOK {
			t.Fatalf("list %s: %d %s", tc.status, rec.Code, rec.Body.String())
		}
		var out struct {
			Comments []comment `json:"comments"`
		}
		decodeJSON(t, rec, &out)
		if len(out.Comments) != tc.expect {
			t.Fatalf("status=%s: expected %d, got %d", tc.status, tc.expect, len(out.Comments))
		}
	}
}

func equalUnorderedStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	count := map[string]int{}
	for _, s := range a {
		count[s]++
	}
	for _, s := range b {
		count[s]--
		if count[s] < 0 {
			return false
		}
	}
	return true
}

func TestListSitesIncludesOpenCommentCount(t *testing.T) {
	f := seedCommentsFixture(t)
	ctx := context.Background()

	// Post two top-level comments; one resolved.
	body1 := `{"pagePath":"/","body":"a"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body1, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	var first struct{ Comment comment }
	decodeJSON(t, rec, &first)

	body2 := `{"pagePath":"/","body":"b"}`
	req = authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body2, f.other)
	rec = httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)

	// Resolve the first one.
	req = authedRequest(t, f.app, "POST", "/api/comments/"+first.Comment.ID+"/resolve", "", f.owner)
	rec = httptest.NewRecorder()
	f.app.commentByIDHandler(rec, req)

	// listSites should show open_comment_count = 1.
	sites, err := f.app.listSites(ctx, f.orgID, listSitesOpts{})
	if err != nil {
		t.Fatalf("listSites: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}
	if sites[0].OpenCommentCount != 1 {
		t.Fatalf("expected openCommentCount=1, got %d", sites[0].OpenCommentCount)
	}
}

// Body trimming + empty body rejection.
func TestCreateComment_RejectsEmptyBody(t *testing.T) {
	f := seedCommentsFixture(t)

	for _, body := range []string{`{"pagePath":"/","body":""}`, `{"pagePath":"/","body":"   "}`} {
		req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
		rec := httptest.NewRecorder()
		f.app.createSiteCommentHandler(rec, req, f.siteID)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("empty body should 400, got %d %s", rec.Code, rec.Body.String())
		}
	}
}

func TestCreateComment_DefaultsDeployToCurrent(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","body":"no deploy"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d %s", rec.Code, rec.Body.String())
	}
	var out struct{ Comment comment }
	decodeJSON(t, rec, &out)
	if out.Comment.DeployID != f.deployID {
		t.Fatalf("expected deployId=%s, got %s", f.deployID, out.Comment.DeployID)
	}
}

// Sanity: invalid status query returns 400.
func TestListComments_InvalidStatus(t *testing.T) {
	f := seedCommentsFixture(t)

	req := authedRequest(t, f.app, "GET", "/api/sites/"+f.siteID+"/comments?status=lol", "", f.owner)
	rec := httptest.NewRecorder()
	f.app.listSiteCommentsHandler(rec, req, f.siteID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// Smoke: outsider gets 403 on list too.
func TestListComments_OutsiderForbidden(t *testing.T) {
	f := seedCommentsFixture(t)

	req := authedRequest(t, f.app, "GET", "/api/sites/"+f.siteID+"/comments", "", f.outsider)
	rec := httptest.NewRecorder()
	f.app.listSiteCommentsHandler(rec, req, f.siteID)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// Body is also reflected in the GET response.
func TestListComments_ReturnsAuthor(t *testing.T) {
	f := seedCommentsFixture(t)

	body := `{"pagePath":"/","body":"hi"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)

	req = authedRequest(t, f.app, "GET", "/api/sites/"+f.siteID+"/comments", "", f.owner)
	rec = httptest.NewRecorder()
	f.app.listSiteCommentsHandler(rec, req, f.siteID)
	var out struct {
		Comments []comment `json:"comments"`
	}
	decodeJSON(t, rec, &out)
	if len(out.Comments) != 1 || out.Comments[0].Author == nil || out.Comments[0].Author.Username != "other" {
		t.Fatalf("expected one comment by 'other', got %+v", out.Comments)
	}
}

// Prevent silent reuse of "other" subscription on outsider posts (smoke).
func TestCreateComment_OutsiderDoesNotPollute(t *testing.T) {
	f := seedCommentsFixture(t)
	ctx := context.Background()

	body := `{"pagePath":"/","body":"member"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)

	body = `{"pagePath":"/","body":"outsider"}`
	req = authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.outsider)
	rec = httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("outsider post: expected 403, got %d", rec.Code)
	}

	// Confirm only one comment exists.
	var n int
	if err := f.app.db.QueryRow(ctx, `select count(*) from comments where site_id = $1`, f.siteID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected 1 comment, got %d err=%v", n, err)
	}
}

// Notifications: mark-read is idempotent and scoped to recipient.
func TestMarkNotificationRead_OtherUserCannotMarkOursRead(t *testing.T) {
	f := seedCommentsFixture(t)
	ctx := context.Background()

	// Seed a notification: other commented top, admin replies → owner gets a notification.
	body := `{"pagePath":"/","body":"top"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	var top struct{ Comment comment }
	decodeJSON(t, rec, &top)

	replyBody := `{"pagePath":"/","parentId":"` + top.Comment.ID + `","body":"reply"}`
	req = authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", replyBody, f.admin)
	rec = httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)

	var notifID string
	if err := f.app.db.QueryRow(ctx, `select id from notifications where recipient_id = $1`, f.owner).Scan(&notifID); err != nil {
		t.Fatalf("find notif: %v", err)
	}

	// other (not owner) tries to mark it read.
	req = authedRequest(t, f.app, "POST", "/api/notifications/"+notifID+"/read", "", f.other)
	rec = httptest.NewRecorder()
	f.app.notificationByIDHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-owner, got %d", rec.Code)
	}

	// owner can mark it read.
	req = authedRequest(t, f.app, "POST", "/api/notifications/"+notifID+"/read", "", f.owner)
	rec = httptest.NewRecorder()
	f.app.notificationByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner mark read: %d %s", rec.Code, rec.Body.String())
	}

	// Verify read_at set.
	var hasRead bool
	if err := f.app.db.QueryRow(ctx, `select read_at is not null from notifications where id = $1`, notifID).Scan(&hasRead); err != nil || !hasRead {
		t.Fatalf("expected read_at set, got read=%v err=%v", hasRead, err)
	}
}

// Mark-all-read scopes to caller's notifications only.
func TestMarkAllNotificationsRead(t *testing.T) {
	f := seedCommentsFixture(t)
	ctx := context.Background()

	body := `{"pagePath":"/","body":"top"}`
	req := authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", body, f.other)
	rec := httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)
	var top struct{ Comment comment }
	decodeJSON(t, rec, &top)

	replyBody := `{"pagePath":"/","parentId":"` + top.Comment.ID + `","body":"r"}`
	req = authedRequest(t, f.app, "POST", "/api/sites/"+f.siteID+"/comments", replyBody, f.admin)
	rec = httptest.NewRecorder()
	f.app.createSiteCommentHandler(rec, req, f.siteID)

	req = authedRequest(t, f.app, "POST", "/api/notifications/read-all", "", f.owner)
	rec = httptest.NewRecorder()
	f.app.notificationByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read-all: %d %s", rec.Code, rec.Body.String())
	}

	var unread int
	if err := f.app.db.QueryRow(ctx, `select count(*) from notifications where recipient_id = $1 and read_at is null`, f.owner).Scan(&unread); err != nil || unread != 0 {
		t.Fatalf("expected 0 unread, got %d err=%v", unread, err)
	}
}

// Compilation guard: ensures the comment runtime is non-empty.
func TestCommentRuntimeNotEmpty(t *testing.T) {
	if !strings.Contains(commentRuntimeJS, "__protopenCommentsLoaded") {
		t.Fatalf("comment runtime missing sentinel")
	}
}
