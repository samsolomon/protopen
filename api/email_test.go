// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestBuildMIME(t *testing.T) {
	withReplyTo := string(buildMIME("from@test", []string{"a@test", "b@test"},
		"reply+tok@reply.test", "Hello", "<p>hi</p>", "hi"))

	for _, want := range []string{
		"From: from@test\r\n",
		"To: a@test, b@test\r\n",
		"Reply-To: reply+tok@reply.test\r\n",
		"Subject: Hello\r\n",
		"Content-Type: multipart/alternative; boundary=",
		"Content-Type: text/plain; charset=\"utf-8\"",
		"Content-Type: text/html; charset=\"utf-8\"",
		"<p>hi</p>",
	} {
		if !strings.Contains(withReplyTo, want) {
			t.Errorf("MIME message missing %q\n---\n%s", want, withReplyTo)
		}
	}

	noReplyTo := string(buildMIME("from@test", []string{"a@test"}, "", "Hi", "<p>x</p>", "x"))
	if strings.Contains(noReplyTo, "Reply-To:") {
		t.Errorf("empty replyTo should omit the header:\n%s", noReplyTo)
	}

	// Non-ASCII subjects are RFC 2047 encoded so they survive transit.
	encoded := string(buildMIME("from@test", []string{"a@test"}, "", "Café résumé", "<p>x</p>", "x"))
	if strings.Contains(encoded, "Subject: Café") {
		t.Errorf("non-ASCII subject should be encoded:\n%s", encoded)
	}
}

// fakeTransport records every send so tests can assert recipients without a
// real mail server.
type fakeTransport struct {
	mu   sync.Mutex
	sent []fakeEmail
}

type fakeEmail struct {
	to      []string
	replyTo string
	subject string
}

func (f *fakeTransport) send(from string, to []string, replyTo, subject, html, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, fakeEmail{to: to, replyTo: replyTo, subject: subject})
	return nil
}

func (f *fakeTransport) recipients() map[string]fakeEmail {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]fakeEmail{}
	for _, e := range f.sent {
		for _, addr := range e.to {
			out[addr] = e
		}
	}
	return out
}

// fanOutFixture seeds a site with four recipients in distinct states.
type fanOutFixture struct {
	app        *application
	fake       *fakeTransport
	siteID     string
	deployID   string
	recipients map[string]string
}

func seedFanOutFixture(t *testing.T) fanOutFixture {
	t.Helper()
	app := newTestApp(t)
	fake := &fakeTransport{}
	app.mailer.Store(newEmailClient(fake, "Protopen <noreply@test>"))

	orgID, siteID, deployID := "org_f", "site_f", "dep_f"
	seedOrg(t, app.db, orgID, "fan", "Fan", false)
	owner := "usr_owner"
	seedUser(t, app.db, owner, "owner@test", "owner", "Owner")
	seedOrgMember(t, app.db, orgID, owner, roleMember)
	seedSite(t, app.db, siteID, orgID, "fan-site", "Fan Site", &owner)
	seedDeploy(t, app, siteID, deployID)

	// verified + default prefs -> emailed; unverified -> skipped;
	// verified but opted out of replies -> skipped; mentioned + verified -> emailed.
	users := []struct{ id, email string }{
		{"usr_verified", "verified@test"},
		{"usr_unverified", "unverified@test"},
		{"usr_optedout", "optedout@test"},
		{"usr_mentioned", "mentioned@test"},
	}
	for _, u := range users {
		seedUser(t, app.db, u.id, u.email, u.id, u.id)
		seedOrgMember(t, app.db, orgID, u.id, roleMember)
	}
	mustExec(t, app.db, `update users set email_verified_at = now()
		where id in ('usr_verified', 'usr_optedout', 'usr_mentioned')`)
	mustExec(t, app.db, `insert into user_notification_prefs (user_id, email_on_reply, email_on_mention)
		values ('usr_optedout', false, true)`)

	return fanOutFixture{
		app: app, fake: fake, siteID: siteID, deployID: deployID,
		recipients: map[string]string{
			"usr_verified":   notificationTypeCommentReply,
			"usr_unverified": notificationTypeCommentReply,
			"usr_optedout":   notificationTypeCommentReply,
			"usr_mentioned":  notificationTypeCommentMention,
		},
	}
}

func TestFanOutCommentEmails_GatesByVerificationAndPrefs(t *testing.T) {
	f := seedFanOutFixture(t)

	mustExec(t, f.app.db, `
		insert into comments (id, site_id, deploy_id, user_id, page_path, body, created_at, updated_at)
		values ('cm_f', $1, $2, 'usr_owner', '/', 'root', now(), now())
	`, f.siteID, f.deployID)

	f.app.fanOutCommentEmails(f.recipients, "Actor", f.siteID, "/", "cm_f", "cm_f", "a comment")

	got := f.fake.recipients()
	if _, ok := got["verified@test"]; !ok {
		t.Error("verified recipient should have been emailed")
	}
	if _, ok := got["mentioned@test"]; !ok {
		t.Error("mentioned recipient should have been emailed")
	}
	if _, ok := got["unverified@test"]; ok {
		t.Error("unverified recipient must not be emailed")
	}
	if _, ok := got["optedout@test"]; ok {
		t.Error("opted-out recipient must not be emailed")
	}
}

func TestFanOutCommentEmails_SetsReplyToWhenInboundConfigured(t *testing.T) {
	f := seedFanOutFixture(t)
	mustExec(t, f.app.db, `
		insert into comments (id, site_id, deploy_id, user_id, page_path, body, created_at, updated_at)
		values ('cm_f', $1, $2, 'usr_owner', '/', 'root', now(), now())
	`, f.siteID, f.deployID)
	mustExec(t, f.app.db, `insert into instance_settings (key, value) values
		('email_inbound_domain', 'reply.test'), ('email_inbound_secret', 'sek')`)

	f.app.fanOutCommentEmails(f.recipients, "Actor", f.siteID, "/", "cm_f", "cm_f", "a comment")

	got := f.fake.recipients()
	mail, ok := got["verified@test"]
	if !ok {
		t.Fatal("verified recipient should have been emailed")
	}
	if !strings.HasPrefix(mail.replyTo, "reply+") || !strings.HasSuffix(mail.replyTo, "@reply.test") {
		t.Errorf("expected reply+<token>@reply.test, got %q", mail.replyTo)
	}
	// The token is stored hashed against the thread root.
	var count int
	if err := f.app.db.QueryRow(context.Background(),
		`select count(*) from comment_reply_tokens where root_comment_id = 'cm_f'`).Scan(&count); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if count == 0 {
		t.Error("expected reply tokens to be minted")
	}
}
