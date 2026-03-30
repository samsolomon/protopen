CREATE TABLE comment_subscriptions (
    id         text PRIMARY KEY,
    comment_id text NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (comment_id, user_id)
);

CREATE TABLE notifications (
    id           text PRIMARY KEY,
    recipient_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type         text NOT NULL,
    comment_id   text REFERENCES comments(id) ON DELETE CASCADE,
    read_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notifications_recipient_idx
    ON notifications (recipient_id, created_at DESC);
CREATE INDEX notifications_recipient_unread_idx
    ON notifications (recipient_id, created_at DESC) WHERE read_at IS NULL;
CREATE INDEX comment_subscriptions_comment_idx
    ON comment_subscriptions (comment_id);
