-- 0007_users.sql (SQLite)
--
-- Customer accounts. Mirrors the Postgres schema; see
-- postgres/0007_users.sql for design notes.

CREATE TABLE IF NOT EXISTS users (
    id             TEXT PRIMARY KEY,
    email          TEXT NOT NULL,
    email_lower    TEXT NOT NULL,
    password_hash  TEXT NOT NULL DEFAULT '',
    google_sub     TEXT,
    name           TEXT NOT NULL DEFAULT '',
    email_verified INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMP NOT NULL,
    updated_at     TIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_lower ON users(email_lower);
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_google_sub ON users(google_sub)
    WHERE google_sub IS NOT NULL;

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
