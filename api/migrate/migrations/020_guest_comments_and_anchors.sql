-- Guest authorship: when user_id is NULL, guest_name carries the display name.
-- Postgres FKs exempt NULL from the constraint, so the existing FK on user_id
-- stays in place; only the NOT NULL needs dropping.
ALTER TABLE comments
    ALTER COLUMN user_id DROP NOT NULL,
    ADD COLUMN guest_name        text,
    ADD COLUMN guest_fingerprint text,
    ADD COLUMN element_selector  text,
    ADD COLUMN element_offset_x  numeric(7,4),
    ADD COLUMN element_offset_y  numeric(7,4),
    ADD CONSTRAINT comments_author_present
        CHECK (user_id IS NOT NULL OR (guest_name IS NOT NULL AND length(guest_name) > 0));

-- Allow guest actors in notifications (NULL actor_id => guest).
ALTER TABLE notifications ALTER COLUMN actor_id DROP NOT NULL;

-- Site-level subscriptions (scope wider than per-thread comment_subscriptions).
-- Site owner is auto-subscribed at site creation; future enhancement can let
-- non-owners subscribe to a whole site too.
CREATE TABLE site_subscriptions (
    id         text PRIMARY KEY,
    site_id    text NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (site_id, user_id)
);

CREATE INDEX site_subscriptions_site_idx ON site_subscriptions (site_id);

-- Backfill: every existing site owner subscribed to their own site.
INSERT INTO site_subscriptions (id, site_id, user_id, created_at)
SELECT 'ssub_' || substr(md5(random()::text || s.id), 1, 16), s.id, s.created_by, now()
FROM sites s
WHERE s.created_by IS NOT NULL
ON CONFLICT (site_id, user_id) DO NOTHING;
