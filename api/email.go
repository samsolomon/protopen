// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"log"
	"mime"
	"net/smtp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/resend/resend-go/v2"
)

const (
	tokenTypeVerify = "verify"
	tokenTypeReset  = "reset"
)

//go:embed email_templates/*.html
var emailTemplatesFS embed.FS

// emailTransport is the wire layer: it knows how to put a built message on
// the network. emailClient sits above it and owns templates + subject lines.
// replyTo is optional ("" omits the header).
type emailTransport interface {
	send(from string, to []string, replyTo, subject, html, text string) error
}

type emailClient struct {
	transport emailTransport
	from      string
	tmpls     *template.Template
}

func newEmailClient(transport emailTransport, from string) *emailClient {
	tmpls := template.Must(template.ParseFS(emailTemplatesFS, "email_templates/*.html"))
	return &emailClient{
		transport: transport,
		from:      from,
		tmpls:     tmpls,
	}
}

// resendTransport sends through the Resend HTTP API.
type resendTransport struct {
	client *resend.Client
}

func newResendTransport(apiKey string) emailTransport {
	return &resendTransport{client: resend.NewClient(apiKey)}
}

func (rt *resendTransport) send(from string, to []string, replyTo, subject, html, text string) error {
	req := &resend.SendEmailRequest{
		From:    from,
		To:      to,
		Subject: subject,
		Html:    html,
		Text:    text,
	}
	if replyTo != "" {
		req.ReplyTo = replyTo
	}
	_, err := rt.client.Emails.Send(req)
	return err
}

// smtpTransport sends through any SMTP server. useTLS selects implicit TLS
// (port 465); otherwise SendMail negotiates STARTTLS opportunistically.
type smtpTransport struct {
	host, port, username, password string
	useTLS                         bool
}

func (st *smtpTransport) send(from string, to []string, replyTo, subject, html, text string) error {
	addr := st.host + ":" + st.port
	msg := buildMIME(from, to, replyTo, subject, html, text)
	var auth smtp.Auth
	if st.username != "" {
		auth = smtp.PlainAuth("", st.username, st.password, st.host)
	}
	if !st.useTLS {
		return smtp.SendMail(addr, auth, from, to, msg)
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: st.host})
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, st.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// buildMIME assembles a multipart/alternative message with CRLF line endings.
// Subjects are RFC 2047 encoded so non-ASCII author names survive.
func buildMIME(from string, to []string, replyTo, subject, html, text string) []byte {
	const boundary = "protopen-mime-boundary"
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	if replyTo != "" {
		b.WriteString("Reply-To: " + replyTo + "\r\n")
	}
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n")
	b.WriteString(text + "\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n\r\n")
	b.WriteString(html + "\r\n")
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

func (app *application) trySendEmail(desc string, to string, fn func(*emailClient) error) {
	m := app.mailer.Load()
	if m == nil {
		log.Printf("email skipped (%s) to=%s", desc, to)
		return
	}
	if err := fn(m); err != nil {
		log.Printf("email failed (%s) to=%s: %v", desc, to, err)
	}
}

func (ec *emailClient) render(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := ec.tmpls.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("render template %s: %w", name, err)
	}
	return buf.String(), nil
}

// sendVerifyEmail sends an email verification link to a new user.
func (ec *emailClient) sendVerifyEmail(to string, name string, verifyURL string) error {
	html, err := ec.render("verify_email.html", map[string]string{
		"Name":      name,
		"VerifyURL": verifyURL,
	})
	if err != nil {
		return err
	}

	return ec.transport.send(ec.from, []string{to}, "", "Verify your Protopen account", html,
		fmt.Sprintf("Hi %s,\n\nVerify your email to get started with Protopen:\n%s\n\nThis link expires in 24 hours.", name, verifyURL))
}

// sendOrgInvite sends an invitation to join an organization.
func (ec *emailClient) sendOrgInvite(to string, orgName string, inviterName string, signupURL string) error {
	html, err := ec.render("org_invite.html", map[string]string{
		"OrgName":     orgName,
		"InviterName": inviterName,
		"SignupURL":   signupURL,
	})
	if err != nil {
		return err
	}

	return ec.transport.send(ec.from, []string{to}, "",
		fmt.Sprintf("%s invited you to %s on Protopen", inviterName, orgName), html,
		fmt.Sprintf("%s invited you to %s on Protopen.\n\nSign up to join:\n%s", inviterName, orgName, signupURL))
}

func createEmailToken(ctx context.Context, db *pgxpool.Pool, userID string, tokenType string, interval string) (string, error) {
	token := generateToken(32)
	_, err := db.Exec(ctx, `
		insert into email_tokens (id, user_id, token_hash, type, created_at, expires_at)
		values ($1, $2, $3, $4, now(), now() + $5::interval)
	`, generateID("etk"), userID, hashToken(token), tokenType, interval)
	if err != nil {
		return "", err
	}
	return token, nil
}

func consumeEmailToken(ctx context.Context, db *pgxpool.Pool, rawToken string, tokenType string) (string, error) {
	hashed := hashToken(rawToken)
	var userID string
	err := db.QueryRow(ctx, `
		update email_tokens set used_at = now()
		where token_hash = $1 and type = $2 and expires_at > now() and used_at is null
		returning user_id
	`, hashed, tokenType).Scan(&userID)
	return userID, err
}

// sendPasswordReset sends a password reset link.
func (ec *emailClient) sendPasswordReset(to string, resetURL string) error {
	html, err := ec.render("password_reset.html", map[string]string{
		"ResetURL": resetURL,
	})
	if err != nil {
		return err
	}

	return ec.transport.send(ec.from, []string{to}, "", "Reset your Protopen password", html,
		fmt.Sprintf("You requested a password reset for your Protopen account.\n\nReset your password:\n%s\n\nThis link expires in 1 hour. If you didn't request this, you can ignore this email.", resetURL))
}

// sendCommentNotification tells a thread participant about new comment
// activity. isMention switches the copy between a reply and an @mention.
// replyTo, when set, is the per-recipient reply-by-email address.
func (ec *emailClient) sendCommentNotification(to, replyTo, actorName, siteName, snippet, threadURL string, isMention bool) error {
	html, err := ec.render("comment_notification.html", map[string]any{
		"ActorName": actorName,
		"SiteName":  siteName,
		"Snippet":   snippet,
		"ThreadURL": threadURL,
		"IsMention": isMention,
	})
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("%s replied on %s", actorName, siteName)
	verb := "replied on a thread you're following on"
	if isMention {
		subject = fmt.Sprintf("%s mentioned you on %s", actorName, siteName)
		verb = "mentioned you in a comment on"
	}
	text := fmt.Sprintf("%s %s %s.\n\n%s\n\nView the thread:\n%s", actorName, verb, siteName, snippet, threadURL)
	return ec.transport.send(ec.from, []string{to}, replyTo, subject, html, text)
}

// sendTestEmail delivers a plain confirmation message so an admin can verify
// the configured provider works.
func (ec *emailClient) sendTestEmail(to string) error {
	const msg = "This is a test email from Protopen. Your email provider is configured correctly."
	html := "<p>" + msg + "</p>"
	return ec.transport.send(ec.from, []string{to}, "", "Protopen email test", html, msg)
}
