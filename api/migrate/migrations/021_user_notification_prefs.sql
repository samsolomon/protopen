-- Per-user notification preferences. An absent row means both defaults
-- (opted in) — no backfill needed; getNotificationPrefs returns the defaults.
CREATE TABLE user_notification_prefs (
    user_id          text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    email_on_reply   boolean NOT NULL DEFAULT true,
    email_on_mention boolean NOT NULL DEFAULT true,
    updated_at       timestamptz NOT NULL DEFAULT now()
);
