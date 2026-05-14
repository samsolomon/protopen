// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/resend/resend-go/v2"
)

const (
	tokenTypeVerify = "verify"
	tokenTypeReset  = "reset"
)

//go:embed email_templates/*.html
var emailTemplatesFS embed.FS

type emailClient struct {
	client *resend.Client
	from   string
	tmpls  *template.Template
}

func newEmailClient(apiKey string, from string) *emailClient {
	tmpls := template.Must(template.ParseFS(emailTemplatesFS, "email_templates/*.html"))
	return &emailClient{
		client: resend.NewClient(apiKey),
		from:   from,
		tmpls:  tmpls,
	}
}

func (app *application) trySendEmail(desc string, to string, fn func() error) {
	if app.mailer == nil {
		log.Printf("email skipped (%s) to=%s", desc, to)
		return
	}
	if err := fn(); err != nil {
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

	_, err = ec.client.Emails.Send(&resend.SendEmailRequest{
		From:    ec.from,
		To:      []string{to},
		Subject: "Verify your Protopen account",
		Html:    html,
		Text:    fmt.Sprintf("Hi %s,\n\nVerify your email to get started with Protopen:\n%s\n\nThis link expires in 24 hours.", name, verifyURL),
	})
	return err
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

	_, err = ec.client.Emails.Send(&resend.SendEmailRequest{
		From:    ec.from,
		To:      []string{to},
		Subject: fmt.Sprintf("%s invited you to %s on Protopen", inviterName, orgName),
		Html:    html,
		Text:    fmt.Sprintf("%s invited you to %s on Protopen.\n\nSign up to join:\n%s", inviterName, orgName, signupURL),
	})
	return err
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

	_, err = ec.client.Emails.Send(&resend.SendEmailRequest{
		From:    ec.from,
		To:      []string{to},
		Subject: "Reset your Protopen password",
		Html:    html,
		Text:    fmt.Sprintf("You requested a password reset for your Protopen account.\n\nReset your password:\n%s\n\nThis link expires in 1 hour. If you didn't request this, you can ignore this email.", resetURL),
	})
	return err
}
