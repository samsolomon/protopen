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
	settingThumbnailsEnabled = "thumbnails_enabled"
	settingEmailProvider     = "email_provider"
	settingEmailFrom         = "email_from"
	settingEmailResendKey    = "email_resend_key"
	settingEmailSMTPHost     = "email_smtp_host"
	settingEmailSMTPPort     = "email_smtp_port"
	settingEmailSMTPUser     = "email_smtp_user"
	settingEmailSMTPPass     = "email_smtp_pass"
	settingEmailSMTPTLS      = "email_smtp_tls"
	settingEmailInboundDomain = "email_inbound_domain"
	settingEmailInboundSecret = "email_inbound_secret"
)

// emailInboundConfig reports the reply-by-email settings. enabled is true only
// when both the inbound domain and the webhook secret are configured.
func (app *application) emailInboundConfig(ctx context.Context) (domain, secret string, enabled bool) {
	domain, _, _ = app.getInstanceSetting(ctx, settingEmailInboundDomain)
	secret, _, _ = app.getInstanceSetting(ctx, settingEmailInboundSecret)
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
	provider, _, _ := app.getInstanceSetting(ctx, settingEmailProvider)
	from, _, _ := app.getInstanceSetting(ctx, settingEmailFrom)
	var transport emailTransport
	switch provider {
	case "resend":
		if key, _, _ := app.getInstanceSetting(ctx, settingEmailResendKey); key != "" {
			transport = newResendTransport(key)
		}
	case "smtp":
		host, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPHost)
		port, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPPort)
		if host != "" && port != "" {
			user, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPUser)
			pass, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPPass)
			tls, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPTLS)
			transport = &smtpTransport{host: host, port: port, username: user, password: pass, useTLS: tls == "true"}
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
		provider := "none"
		if key := getenv("RESEND_API_KEY", ""); key != "" {
			provider = "resend"
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
	provider, _, _ := app.getInstanceSetting(ctx, settingEmailProvider)
	if provider == "" {
		provider = "none"
	}
	from, _, _ := app.getInstanceSetting(ctx, settingEmailFrom)
	resendKey, _, _ := app.getInstanceSetting(ctx, settingEmailResendKey)
	smtpHost, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPHost)
	smtpPort, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPPort)
	smtpUser, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPUser)
	smtpPass, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPPass)
	smtpTLS, _, _ := app.getInstanceSetting(ctx, settingEmailSMTPTLS)
	inboundDomain, _, _ := app.getInstanceSetting(ctx, settingEmailInboundDomain)
	inboundSecret, _, _ := app.getInstanceSetting(ctx, settingEmailInboundSecret)
	return adminSettingsResponse{
		Thumbnails: adminThumbnailSettings{
			Available: app.thumbnailEnv.available,
			Enabled:   app.thumbnailer.Load() != nil,
			Reason:    app.thumbnailEnv.reason,
		},
		Email: adminEmailSettings{
			Provider:         provider,
			From:             from,
			ResendKeySet:     resendKey != "",
			SMTPHost:         smtpHost,
			SMTPPort:         smtpPort,
			SMTPUser:         smtpUser,
			SMTPPassSet:      smtpPass != "",
			SMTPTLS:          smtpTLS == "true",
			InboundDomain:    inboundDomain,
			InboundSecretSet: inboundSecret != "",
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
		ok := true
		if e.Provider != nil {
			ok = ok && set(settingEmailProvider, *e.Provider)
		}
		if ok && e.From != nil {
			ok = ok && set(settingEmailFrom, *e.From)
		}
		if ok && e.SMTPHost != nil {
			ok = ok && set(settingEmailSMTPHost, *e.SMTPHost)
		}
		if ok && e.SMTPPort != nil {
			ok = ok && set(settingEmailSMTPPort, *e.SMTPPort)
		}
		if ok && e.SMTPUser != nil {
			ok = ok && set(settingEmailSMTPUser, *e.SMTPUser)
		}
		if ok && e.SMTPTLS != nil {
			ok = ok && set(settingEmailSMTPTLS, strconv.FormatBool(*e.SMTPTLS))
		}
		if ok && e.InboundDomain != nil {
			ok = ok && set(settingEmailInboundDomain, *e.InboundDomain)
		}
		if ok && e.ResendKey != nil && *e.ResendKey != "" {
			ok = ok && set(settingEmailResendKey, *e.ResendKey)
		}
		if ok && e.SMTPPass != nil && *e.SMTPPass != "" {
			ok = ok && set(settingEmailSMTPPass, *e.SMTPPass)
		}
		if ok && e.InboundSecret != nil && *e.InboundSecret != "" {
			ok = ok && set(settingEmailInboundSecret, *e.InboundSecret)
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
