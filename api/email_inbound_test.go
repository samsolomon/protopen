// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripQuotedReply(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "Looks good to me.", "Looks good to me."},
		{"trims whitespace", "  hello  \n\n", "hello"},
		{"attribution line", "Ship it.\n\nOn Wed, May 20 2026, Demo User wrote:\n> original", "Ship it."},
		{"quote block", "Agreed.\n> what about this\n> and this", "Agreed."},
		{"signature delimiter", "Done.\n--\nJane Chen", "Done."},
		{"crlf endings", "First line.\r\nSecond line.\r\n", "First line.\nSecond line."},
		{"empty input", "", ""},
		{"fully quoted", "> nothing but a quote", ""},
		{"multiline body kept", "Line one.\nLine two.\n\nOn Mon, X wrote:\n> q", "Line one.\nLine two."},
		{"attribution is case-insensitive", "ok\non tue, x WROTE:\n> q", "ok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripQuotedReply(c.in); got != c.want {
				t.Errorf("stripQuotedReply(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestExtractReplyToken(t *testing.T) {
	const domain = "reply.example.com"
	cases := []struct {
		name string
		to   string
		want string
	}{
		{"bare address", "reply+abc123@reply.example.com", "abc123"},
		{"display-name form", "Jane Chen <reply+abc123@reply.example.com>", "abc123"},
		{"uppercase domain", "reply+abc123@REPLY.EXAMPLE.COM", "abc123"},
		{"wrong domain", "reply+abc123@other.example.com", ""},
		{"missing reply+ prefix", "support+abc123@reply.example.com", ""},
		{"plain mailbox", "support@reply.example.com", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractReplyToken(c.to, domain); got != c.want {
				t.Errorf("extractReplyToken(%q) = %q, want %q", c.to, got, c.want)
			}
		})
	}
}

// inboundFixture extends the comments fixture with reply-by-email config, a
// root comment, and a reply token for the "owner" user.
type inboundFixture struct {
	commentsFixture
	rootID   string
	rawToken string
	secret   string
}

func seedInboundFixture(t *testing.T) inboundFixture {
	t.Helper()
	f := seedCommentsFixture(t)
	const secret = "test-webhook-secret"
	mustExec(t, f.app.db, `
		insert into instance_settings (key, value) values
			('email_inbound_domain', 'reply.test'),
			('email_inbound_secret', $1)
	`, secret)

	rootID := "cm_root"
	mustExec(t, f.app.db, `
		insert into comments (id, site_id, deploy_id, user_id, page_path, body, created_at, updated_at)
		values ($1, $2, $3, $4, '/', 'root comment', now(), now())
	`, rootID, f.siteID, f.deployID, f.owner)

	raw := "abc123def456abc123def456abc12345"
	mustExec(t, f.app.db, `
		insert into comment_reply_tokens (token, root_comment_id, user_id, expires_at)
		values ($1, $2, $3, now() + interval '1 day')
	`, hashToken(raw), rootID, f.owner)

	return inboundFixture{commentsFixture: f, rootID: rootID, rawToken: raw, secret: secret}
}

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func inboundRequest(body []byte, signature string) *http.Request {
	req := httptest.NewRequest("POST", "/api/email/inbound", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Protopen-Signature", signature)
	}
	return req
}

func TestEmailInbound_PostsReply(t *testing.T) {
	f := seedInboundFixture(t)

	payload, _ := json.Marshal(map[string]string{
		"to":   "reply+" + f.rawToken + "@reply.test",
		"from": "Owner <owner@example.test>",
		"text": "Replying by email.\n\nOn Wed, X wrote:\n> root comment",
	})
	rec := httptest.NewRecorder()
	f.app.emailInboundHandler(rec, inboundRequest(payload, signBody(f.secret, payload)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body, author string
	if err := f.app.db.QueryRow(context.Background(),
		`select body, user_id from comments where parent_id = $1`, f.rootID).Scan(&body, &author); err != nil {
		t.Fatalf("reply not inserted: %v", err)
	}
	if body != "Replying by email." {
		t.Errorf("quoted text not stripped: body=%q", body)
	}
	if author != f.owner {
		t.Errorf("reply attributed to %q, want %q", author, f.owner)
	}
}

func TestEmailInbound_BadSignature(t *testing.T) {
	f := seedInboundFixture(t)
	payload, _ := json.Marshal(map[string]string{
		"to": "reply+" + f.rawToken + "@reply.test", "from": "owner@example.test", "text": "hi",
	})
	rec := httptest.NewRecorder()
	f.app.emailInboundHandler(rec, inboundRequest(payload, "deadbeef"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestEmailInbound_FromMismatch(t *testing.T) {
	f := seedInboundFixture(t)
	payload, _ := json.Marshal(map[string]string{
		"to": "reply+" + f.rawToken + "@reply.test", "from": "attacker@evil.test", "text": "posting as owner",
	})
	rec := httptest.NewRecorder()
	f.app.emailInboundHandler(rec, inboundRequest(payload, signBody(f.secret, payload)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmailInbound_UnknownToken(t *testing.T) {
	f := seedInboundFixture(t)
	payload, _ := json.Marshal(map[string]string{
		"to": "reply+nosuchtoken000000000000000000@reply.test", "from": "owner@example.test", "text": "hi",
	})
	rec := httptest.NewRecorder()
	f.app.emailInboundHandler(rec, inboundRequest(payload, signBody(f.secret, payload)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmailInbound_NotConfigured(t *testing.T) {
	f := seedCommentsFixture(t) // no inbound settings
	payload, _ := json.Marshal(map[string]string{"to": "reply+x@reply.test", "from": "a@b.test", "text": "hi"})
	rec := httptest.NewRecorder()
	f.app.emailInboundHandler(rec, inboundRequest(payload, signBody("whatever", payload)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestEmailInbound_EmptyAfterStripIsNoOp(t *testing.T) {
	f := seedInboundFixture(t)
	payload, _ := json.Marshal(map[string]string{
		"to":   "reply+" + f.rawToken + "@reply.test",
		"from": "owner@example.test",
		"text": "> only a quoted line, nothing new",
	})
	rec := httptest.NewRecorder()
	f.app.emailInboundHandler(rec, inboundRequest(payload, signBody(f.secret, payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 no-op, got %d body=%s", rec.Code, rec.Body.String())
	}
	var count int
	if err := f.app.db.QueryRow(context.Background(),
		`select count(*) from comments where parent_id = $1`, f.rootID).Scan(&count); err != nil {
		t.Fatalf("count replies: %v", err)
	}
	if count != 0 {
		t.Errorf("expected no reply inserted, got %d", count)
	}
}
