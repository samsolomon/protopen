-- Email verification
ALTER TABLE users ADD COLUMN email_verified_at timestamptz;

-- Backfill: mark all existing users as verified
UPDATE users SET email_verified_at = created_at;

-- Unified token table for email verification and password resets
CREATE TABLE email_tokens (
    id         text PRIMARY KEY,
    user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    type       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);
