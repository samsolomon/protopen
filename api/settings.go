// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	settingThumbnailsEnabled  = "thumbnails_enabled"
	settingEmailProvider      = "email_provider"
	settingEmailFrom          = "email_from"
	settingEmailResendKey     = "email_resend_key"
	settingEmailSMTPHost      = "email_smtp_host"
	settingEmailSMTPPort      = "email_smtp_port"
	settingEmailSMTPUser      = "email_smtp_user"
	settingEmailSMTPPass      = "email_smtp_pass"
	settingEmailSMTPTLS       = "email_smtp_tls"
	settingEmailInboundDomain = "email_inbound_domain"
	settingEmailInboundSecret = "email_inbound_secret"
)

// Email provider values stored under settingEmailProvider.
const (
	emailProviderNone   = "none"
	emailProviderResend = "resend"
	emailProviderSMTP   = "smtp"
)

// emailInboundConfig reports the reply-by-email settings. enabled is true only
// when both the inbound domain and the webhook secret are configured.
func (app *application) emailInboundConfig(ctx context.Context) (domain, secret string, enabled bool) {
	s := app.getInstanceSettings(ctx, settingEmailInboundDomain, settingEmailInboundSecret)
	domain, secret = s[settingEmailInboundDomain], s[settingEmailInboundSecret]
	return domain, secret, domain != "" && secret != ""
}

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

// getInstanceSettings fetches several settings in one query. Missing keys are
// simply absent from the map (callers treat that as the empty string).
func (app *application) getInstanceSettings(ctx context.Context, keys ...string) map[string]string {
	out := make(map[string]string, len(keys))
	rows, err := app.db.Query(ctx, `select key, value from instance_settings where key = any($1)`, keys)
	if err != nil {
		log.Printf("load instance settings: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		out[k] = v
	}
	return out
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
	Email      adminEmailSettings     `json:"email"`
}

type adminThumbnailSettings struct {
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Reason    string `json:"reason,omitempty"`
}

// adminEmailSettings is the client-facing view of the email config. Secrets
// are never sent back — only the *Set booleans report whether one is stored.
type adminEmailSettings struct {
	Provider         string `json:"provider"`
	From             string `json:"from"`
	ResendKeySet     bool   `json:"resendKeySet"`
	SMTPHost         string `json:"smtpHost"`
	SMTPPort         string `json:"smtpPort"`
	SMTPUser         string `json:"smtpUser"`
	SMTPPassSet      bool   `json:"smtpPassSet"`
	SMTPTLS          bool   `json:"smtpTLS"`
	InboundDomain    string `json:"inboundDomain"`
	InboundSecretSet bool   `json:"inboundSecretSet"`
}

// rebuildMailer reads the email config from instance_settings and atomically
// swaps app.mailer. Stores nil (sending disabled) when the provider is none
// or its required fields are missing.
func (app *application) rebuildMailer(ctx context.Context) {
	s := app.getInstanceSettings(ctx,
		settingEmailProvider, settingEmailFrom, settingEmailResendKey,
		settingEmailSMTPHost, settingEmailSMTPPort, settingEmailSMTPUser,
		settingEmailSMTPPass, settingEmailSMTPTLS)
	provider := s[settingEmailProvider]
	from := s[settingEmailFrom]
	var transport emailTransport
	switch provider {
	case emailProviderResend:
		if key := s[settingEmailResendKey]; key != "" {
			transport = newResendTransport(key)
		}
	case emailProviderSMTP:
		host, port := s[settingEmailSMTPHost], s[settingEmailSMTPPort]
		if host != "" && port != "" {
			transport = &smtpTransport{
				host:     host,
				port:     port,
				username: s[settingEmailSMTPUser],
				password: s[settingEmailSMTPPass],
				useTLS:   s[settingEmailSMTPTLS] == "true",
			}
		}
	}
	if transport == nil {
		app.mailer.Store(nil)
		log.Printf("email sending disabled (provider=%q)", provider)
		return
	}
	app.mailer.Store(newEmailClient(transport, from))
	log.Printf("email sending enabled via %s", provider)
}

// initEmailState seeds the email config from env on first boot (so existing
// RESEND_API_KEY deploys keep working), then builds the mailer. After the
// first boot the DB is authoritative and env changes are ignored.
func (app *application) initEmailState(ctx context.Context) error {
	_, exists, err := app.getInstanceSetting(ctx, settingEmailProvider)
	if err != nil {
		return err
	}
	if !exists {
		provider := emailProviderNone
		if key := getenv("RESEND_API_KEY", ""); key != "" {
			provider = emailProviderResend
			if err := app.setInstanceSetting(ctx, settingEmailResendKey, key, "boot"); err != nil {
				return err
			}
			if err := app.setInstanceSetting(ctx, settingEmailFrom, getenv("RESEND_FROM_ADDRESS", ""), "boot"); err != nil {
				return err
			}
		}
		if err := app.setInstanceSetting(ctx, settingEmailProvider, provider, "boot"); err != nil {
			return err
		}
	}
	app.rebuildMailer(ctx)
	return nil
}

func (app *application) instanceSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := app.requireAdmin(w, r); !ok {
			return
		}
		writeJSON(w, http.StatusOK, app.currentSettings(r.Context()))
	case http.MethodPatch:
		app.patchInstanceSettings(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (app *application) currentSettings(ctx context.Context) adminSettingsResponse {
	s := app.getInstanceSettings(ctx,
		settingEmailProvider, settingEmailFrom, settingEmailResendKey,
		settingEmailSMTPHost, settingEmailSMTPPort, settingEmailSMTPUser,
		settingEmailSMTPPass, settingEmailSMTPTLS,
		settingEmailInboundDomain, settingEmailInboundSecret)
	provider := s[settingEmailProvider]
	if provider == "" {
		provider = emailProviderNone
	}
	return adminSettingsResponse{
		Thumbnails: adminThumbnailSettings{
			Available: app.thumbnailEnv.available,
			Enabled:   app.thumbnailer.Load() != nil,
			Reason:    app.thumbnailEnv.reason,
		},
		Email: adminEmailSettings{
			Provider:         provider,
			From:             s[settingEmailFrom],
			ResendKeySet:     s[settingEmailResendKey] != "",
			SMTPHost:         s[settingEmailSMTPHost],
			SMTPPort:         s[settingEmailSMTPPort],
			SMTPUser:         s[settingEmailSMTPUser],
			SMTPPassSet:      s[settingEmailSMTPPass] != "",
			SMTPTLS:          s[settingEmailSMTPTLS] == "true",
			InboundDomain:    s[settingEmailInboundDomain],
			InboundSecretSet: s[settingEmailInboundSecret] != "",
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
		Email             *struct {
			Provider      *string `json:"provider"`
			From          *string `json:"from"`
			ResendKey     *string `json:"resendKey"`
			SMTPHost      *string `json:"smtpHost"`
			SMTPPort      *string `json:"smtpPort"`
			SMTPUser      *string `json:"smtpUser"`
			SMTPPass      *string `json:"smtpPass"`
			SMTPTLS       *bool   `json:"smtpTLS"`
			InboundDomain *string `json:"inboundDomain"`
			InboundSecret *string `json:"inboundSecret"`
		} `json:"email"`
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

	if e := payload.Email; e != nil {
		// Plain fields overwrite on any non-nil value (including ""). Secret
		// fields only overwrite when a non-empty value is supplied, so the
		// write-only UI can submit a blank field to keep the stored secret.
		set := func(key, val string) bool {
			if err := app.setInstanceSetting(r.Context(), key, val, user.Email); err != nil {
				log.Printf("set instance setting %s: %v", key, err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save settings"})
				return false
			}
			return true
		}
		// plain overwrites on any non-nil value; secret only overwrites on a
		// non-empty value, so a blank field keeps the stored secret.
		plain := func(key string, val *string) bool { return val == nil || set(key, *val) }
		secret := func(key string, val *string) bool { return val == nil || *val == "" || set(key, *val) }
		ok := plain(settingEmailProvider, e.Provider) &&
			plain(settingEmailFrom, e.From) &&
			plain(settingEmailSMTPHost, e.SMTPHost) &&
			plain(settingEmailSMTPPort, e.SMTPPort) &&
			plain(settingEmailSMTPUser, e.SMTPUser) &&
			plain(settingEmailInboundDomain, e.InboundDomain) &&
			secret(settingEmailResendKey, e.ResendKey) &&
			secret(settingEmailSMTPPass, e.SMTPPass) &&
			secret(settingEmailInboundSecret, e.InboundSecret)
		if ok && e.SMTPTLS != nil {
			ok = set(settingEmailSMTPTLS, strconv.FormatBool(*e.SMTPTLS))
		}
		if !ok {
			return
		}
		app.rebuildMailer(r.Context())
	}

	writeJSON(w, http.StatusOK, app.currentSettings(r.Context()))
}

// emailTestHandler sends a one-off test message through the current mailer so
// an admin can confirm the provider config works. POST /api/admin/settings/email-test
func (app *application) emailTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, ok := app.requireAdmin(w, r); !ok {
		return
	}
	var payload struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.To) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "recipient required"})
		return
	}
	m := app.mailer.Load()
	if m == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email is not configured"})
		return
	}
	if err := m.sendTestEmail(strings.TrimSpace(payload.To)); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
