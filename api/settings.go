// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	settingThumbnailsEnabled    = "thumbnails_enabled"
	settingEmailProvider        = "email_provider"
	settingEmailFrom            = "email_from"
	settingEmailResendKey       = "email_resend_key"
	settingEmailSMTPHost        = "email_smtp_host"
	settingEmailSMTPPort        = "email_smtp_port"
	settingEmailSMTPUser        = "email_smtp_user"
	settingEmailSMTPPass        = "email_smtp_pass"
	settingEmailSMTPTLS         = "email_smtp_tls"
	settingEmailInboundDomain   = "email_inbound_domain"
	settingEmailInboundSecret   = "email_inbound_secret"
	settingDefaultSitePrivate   = "default_site_private"
	settingAutoPrivateEnabled   = "auto_private_enabled"
	settingAutoPrivateAfterDays = "auto_private_after_days"
)

// Defaults applied when the corresponding instance_settings row is absent.
const (
	defaultAutoPrivateAfterDays = 30
	minAutoPrivateAfterDays     = 1
	maxAutoPrivateAfterDays     = 3650
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
	Thumbnails adminThumbnailSettings  `json:"thumbnails"`
	Email      adminEmailSettings      `json:"email"`
	Visibility adminVisibilitySettings `json:"visibility"`
}

type adminThumbnailSettings struct {
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Reason    string `json:"reason,omitempty"`
}

// adminVisibilitySettings carries the three site-visibility settings plus a
// preview count of sites currently eligible for the auto-private sweeper. The
// count is rendered in the admin UI next to the toggle so an operator can see
// how many sites would flip before enabling the policy.
type adminVisibilitySettings struct {
	DefaultSitePrivate      bool `json:"defaultSitePrivate"`
	AutoPrivateEnabled      bool `json:"autoPrivateEnabled"`
	AutoPrivateAfterDays    int  `json:"autoPrivateAfterDays"`
	EligibleForRevertCount  int  `json:"eligibleForRevertCount"`
}

// visibilityPolicy is the in-process view of the three visibility settings,
// read on demand from instance_settings. Kept tiny so callers can take a
// fresh snapshot per request/tick without coordination.
type visibilityPolicy struct {
	defaultSitePrivate   bool
	autoPrivateEnabled   bool
	autoPrivateAfterDays int
}

func (app *application) currentVisibilityPolicy(ctx context.Context) visibilityPolicy {
	s := app.getInstanceSettings(ctx,
		settingDefaultSitePrivate, settingAutoPrivateEnabled, settingAutoPrivateAfterDays)
	days := defaultAutoPrivateAfterDays
	if raw := s[settingAutoPrivateAfterDays]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= minAutoPrivateAfterDays && parsed <= maxAutoPrivateAfterDays {
			days = parsed
		}
	}
	return visibilityPolicy{
		defaultSitePrivate:   s[settingDefaultSitePrivate] == "true",
		autoPrivateEnabled:   s[settingAutoPrivateEnabled] == "true",
		autoPrivateAfterDays: days,
	}
}

// countSitesEligibleForRevert mirrors the sweeper's WHERE clause so the admin
// UI can preview how many sites would be flipped. Cheap thanks to the partial
// index added in migration 023, and skipped entirely when the policy is
// disabled (the count is never rendered in that case).
func (app *application) countSitesEligibleForRevert(ctx context.Context, enabled bool, days int) int {
	if !enabled {
		return 0
	}
	var n int
	err := app.db.QueryRow(ctx, `
		select count(*) from sites
		where is_public = true and deleted_at is null and made_public_at is not null
		  and made_public_at < now() - make_interval(days => $1)
	`, days).Scan(&n)
	if err != nil {
		log.Printf("count sites eligible for revert: %v", err)
		return 0
	}
	return n
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

// initVisibilityState seeds the three site-visibility settings from env on
// first boot. Once a row exists, env vars are ignored — the admin UI is the
// source of truth thereafter. Mirrors initEmailState's "seed-once" pattern.
//
// Env vars:
//
//	DEFAULT_VISIBILITY      = "private" | "public"  (default: "public")
//	AUTO_PRIVATE_AFTER_DAYS = integer, clamped to [1, 3650]
func (app *application) initVisibilityState(ctx context.Context) error {
	_, exists, err := app.getInstanceSetting(ctx, settingDefaultSitePrivate)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	defaultPrivate := strings.EqualFold(getenv("DEFAULT_VISIBILITY", "public"), "private")
	if err := app.setInstanceSetting(ctx, settingDefaultSitePrivate, strconv.FormatBool(defaultPrivate), "boot"); err != nil {
		return err
	}

	autoEnabled := false
	days := defaultAutoPrivateAfterDays
	if raw := strings.TrimSpace(getenv("AUTO_PRIVATE_AFTER_DAYS", "")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= minAutoPrivateAfterDays && parsed <= maxAutoPrivateAfterDays {
			days = parsed
			autoEnabled = true
		} else {
			log.Printf("AUTO_PRIVATE_AFTER_DAYS=%q ignored (must be %d-%d)", raw, minAutoPrivateAfterDays, maxAutoPrivateAfterDays)
		}
	}
	if err := app.setInstanceSetting(ctx, settingAutoPrivateEnabled, strconv.FormatBool(autoEnabled), "boot"); err != nil {
		return err
	}
	if err := app.setInstanceSetting(ctx, settingAutoPrivateAfterDays, strconv.Itoa(days), "boot"); err != nil {
		return err
	}
	return nil
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
	policy := app.currentVisibilityPolicy(ctx)
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
		Visibility: adminVisibilitySettings{
			DefaultSitePrivate:     policy.defaultSitePrivate,
			AutoPrivateEnabled:     policy.autoPrivateEnabled,
			AutoPrivateAfterDays:   policy.autoPrivateAfterDays,
			EligibleForRevertCount: app.countSitesEligibleForRevert(ctx, policy.autoPrivateEnabled, policy.autoPrivateAfterDays),
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
		Visibility *struct {
			DefaultSitePrivate   *bool `json:"defaultSitePrivate"`
			AutoPrivateEnabled   *bool `json:"autoPrivateEnabled"`
			AutoPrivateAfterDays *int  `json:"autoPrivateAfterDays"`
		} `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if v := payload.Visibility; v != nil {
		if v.AutoPrivateAfterDays != nil {
			d := *v.AutoPrivateAfterDays
			if d < minAutoPrivateAfterDays || d > maxAutoPrivateAfterDays {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("autoPrivateAfterDays must be between %d and %d", minAutoPrivateAfterDays, maxAutoPrivateAfterDays),
				})
				return
			}
		}
		setVis := func(key, val string) bool {
			if err := app.setInstanceSetting(r.Context(), key, val, user.Email); err != nil {
				log.Printf("set instance setting %s: %v", key, err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save settings"})
				return false
			}
			return true
		}
		if v.DefaultSitePrivate != nil && !setVis(settingDefaultSitePrivate, strconv.FormatBool(*v.DefaultSitePrivate)) {
			return
		}
		if v.AutoPrivateEnabled != nil && !setVis(settingAutoPrivateEnabled, strconv.FormatBool(*v.AutoPrivateEnabled)) {
			return
		}
		if v.AutoPrivateAfterDays != nil && !setVis(settingAutoPrivateAfterDays, strconv.Itoa(*v.AutoPrivateAfterDays)) {
			return
		}
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
