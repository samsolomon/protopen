-- Reply-by-email tokens. A notification email's Reply-To carries one of
-- these; the inbound webhook looks it up to attribute the reply to the
-- right user and thread. Not consumed on use — a thread keeps receiving
-- replies until the token expires (swept by cleanup).
CREATE TABLE comment_reply_tokens (
    token           text PRIMARY KEY,
    root_comment_id text NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    user_id         text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL
);

CREATE INDEX comment_reply_tokens_expiry_idx ON comment_reply_tokens (expires_at);
