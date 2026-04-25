-- 0007_users.sql (Postgres)
--
-- Customer accounts. Supports two auth paths per user:
--  * password_hash populated + google_sub NULL  = email/password signup
--  * password_hash '' + google_sub populated    = Google OAuth signup
--  * both populated                             = user linked both methods
--
-- Email is globally unique (case-insensitive) — one account per email
-- is a product requirement. google_sub is unique when non-NULL so the
-- Google "sub" claim can never map to two different accounts.

CREATE TABLE IF NOT EXISTS users (
    id             TEXT PRIMARY KEY,
    email          TEXT NOT NULL,
    email_lower    TEXT NOT NULL,
    password_hash  TEXT NOT NULL DEFAULT '',
    google_sub     TEXT,
    name           TEXT NOT NULL DEFAULT '',
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMP NOT NULL,
    updated_at     TIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_lower ON users(email_lower);
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_google_sub
    ON users(google_sub) WHERE google_sub IS NOT NULL;

-- Persistent session store. Keeping sessions in the DB (vs. purely
-- stateless HMAC) lets us revoke individual sessions if a user logs
-- out or we detect credential theft.
CREATE TABLE IF NOT EXISTS user_sessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issued_at   TIMESTAMP NOT NULL,
    expires_at  TIMESTAMP NOT NULL,
    revoked_at  TIMESTAMP,
    user_agent  TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at);
