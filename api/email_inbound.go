// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// maxInboundBodyBytes caps the inbound webhook payload. Forwarded emails with
// quoted history run larger than the global 1 MiB JSON limit.
const maxInboundBodyBytes = 10 << 20

var quoteHeaderRe = regexp.MustCompile(`(?i)^On .+ wrote:$`)

// stripQuotedReply trims the quoted original from an email reply, keeping only
// the new text. Heuristic: cut at the first attribution line ("On … wrote:"),
// quote block (`>`), or signature delimiter (`--`).
func stripQuotedReply(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if quoteHeaderRe.MatchString(t) || strings.HasPrefix(t, ">") || t == "--" {
			break
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// extractReplyToken pulls <token> out of a reply+<token>@<domain> address.
func extractReplyToken(to, domain string) string {
	address := strings.TrimSpace(to)
	if addr, err := mail.ParseAddress(to); err == nil {
		address = addr.Address
	}
	suffix := "@" + domain
	if !strings.HasSuffix(strings.ToLower(address), strings.ToLower(suffix)) {
		return ""
	}
	local := address[:len(address)-len(suffix)]
	const prefix = "reply+"
	if !strings.HasPrefix(local, prefix) {
		return ""
	}
	return local[len(prefix):]
}

// emailInboundHandler: POST /api/email/inbound — receives a parsed inbound
// email from the provider's webhook and posts it as a reply on the thread the
// reply token points at. No session auth; gated by the provider signature,
// the reply token, and a From-address match.
func (app *application) emailInboundHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()

	domain, secret, enabled := app.emailInboundConfig(ctx)
	if !enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "inbound email is not configured"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxInboundBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read body"})
		return
	}

	// Verify HMAC-SHA256 of the raw body against the configured secret.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(r.Header.Get("X-Protopen-Signature")), []byte(expected)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid signature"})
		return
	}

	var payload struct {
		To           string `json:"to"`
		From         string `json:"from"`
		Text         string `json:"text"`
		StrippedText string `json:"strippedText"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	token := extractReplyToken(payload.To, domain)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no reply token in recipient address"})
		return
	}

	var rootID, tokenUserID string
	err = app.db.QueryRow(ctx, `
		select root_comment_id, user_id from comment_reply_tokens
		where token = $1 and expires_at > now()
	`, token).Scan(&rootID, &tokenUserID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid or expired reply token"})
		return
	}

	var userEmail, userName string
	if err := app.db.QueryRow(ctx, `select email, name from users where id = $1`, tokenUserID).Scan(&userEmail, &userName); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unknown user"})
		return
	}
	fromAddr, err := mail.ParseAddress(payload.From)
	if err != nil || !strings.EqualFold(fromAddr.Address, userEmail) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "from address does not match the token"})
		return
	}

	// The reply needs the root comment's site/deploy/page context — an inbound
	// email carries none of it.
	var siteID, deployID, pagePath, orgID string
	err = app.db.QueryRow(ctx, `
		select c.site_id, c.deploy_id, c.page_path, p.org_id
		from comments c
		join sites p on p.id = c.site_id
		where c.id = $1 and p.deleted_at is null
	`, rootID).Scan(&siteID, &deployID, &pagePath, &orgID)
	if err != nil {
		writeJSON(w, http.StatusGone, map[string]string{"error": "the thread no longer exists"})
		return
	}

	var isMember bool
	if err := app.db.QueryRow(ctx,
		`select exists(select 1 from org_members where user_id = $1 and org_id = $2)`,
		tokenUserID, orgID).Scan(&isMember); err != nil || !isMember {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no access to this site"})
		return
	}

	body := payload.StrippedText
	if strings.TrimSpace(body) == "" {
		body = stripQuotedReply(payload.Text)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		// Nothing to post (e.g. an empty or fully-quoted reply). Ack so the
		// provider doesn't retry.
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(body) > 8000 {
		body = body[:8000]
	}

	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		log.Printf("inbound email begin tx: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not post reply"})
		return
	}
	defer tx.Rollback(ctx)

	res, err := app.insertComment(ctx, tx, insertCommentParams{
		siteID:   siteID,
		deployID: deployID,
		orgID:    orgID,
		authorID: tokenUserID,
		pagePath: pagePath,
		parentID: &rootID,
		rootID:   rootID,
		body:     body,
	})
	if err != nil {
		log.Printf("inbound email insert comment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not post reply"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		log.Printf("inbound email commit: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not post reply"})
		return
	}

	go app.fanOutCommentEmails(res.recipients, userName, siteID, pagePath, res.rootID, res.commentID, body)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
