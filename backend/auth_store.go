package main

// Persistence layer for customer accounts + their sessions.
//
// The store is the only code that knows the schema of the users and
// user_sessions tables; every other call site goes through these
// methods. Email is deduplicated at the DB level via a UNIQUE index on
// email_lower, so createUser callers simply surface errUserExists to
// the HTTP layer without racing to pre-check.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type userStore struct {
	db *sql.DB
}

func newUserStore(db *sql.DB) *userStore { return &userStore{db: db} }

type userAccount struct {
	ID            string
	Email         string // as entered by the user (for display)
	EmailLower    string // lower-cased key used for lookups
	PasswordHash  string // empty for Google-only users
	GoogleSub     string // empty when not linked
	Name          string
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

var (
	errUserExists   = errors.New("user already exists")
	errUserNotFound = errors.New("user not found")
)

// createUser inserts a new account. The caller must have already
// hashed the password (if any). Returns errUserExists when the email
// (case-insensitive) or google_sub is already taken.
func (s *userStore) createUser(ctx context.Context, u *userAccount) error {
	if u == nil {
		return errors.New("nil user")
	}
	u.EmailLower = strings.ToLower(strings.TrimSpace(u.Email))
	if u.EmailLower == "" {
		return errors.New("empty email")
	}
	if u.ID == "" {
		u.ID = randomID("usr_")
	}
	now := time.Now().UTC()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now

	var gsub any
	if u.GoogleSub != "" {
		gsub = u.GoogleSub
	}
	_, err := s.db.ExecContext(ctx, rb(`INSERT INTO users
		(id, email, email_lower, password_hash, google_sub, name, email_verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		u.ID, u.Email, u.EmailLower, u.PasswordHash, gsub, u.Name,
		boolForDialect(u.EmailVerified), u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return errUserExists
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// getUserByEmail returns the user (if any) matching the given email
// case-insensitively. Returns (nil, errUserNotFound) when no row
// exists.
func (s *userStore) getUserByEmail(ctx context.Context, email string) (*userAccount, error) {
	key := strings.ToLower(strings.TrimSpace(email))
	if key == "" {
		return nil, errUserNotFound
	}
	row := s.db.QueryRowContext(ctx, rb(`SELECT
		id, email, email_lower, password_hash,
		COALESCE(google_sub, ''), name, email_verified, created_at, updated_at
		FROM users WHERE email_lower = ?`), key)
	return scanUser(row)
}

// getUserByID is used by the session middleware to load the current
// user on each authenticated request.
func (s *userStore) getUserByID(ctx context.Context, id string) (*userAccount, error) {
	row := s.db.QueryRowContext(ctx, rb(`SELECT
		id, email, email_lower, password_hash,
		COALESCE(google_sub, ''), name, email_verified, created_at, updated_at
		FROM users WHERE id = ?`), id)
	return scanUser(row)
}

// getUserByGoogleSub matches a Google "sub" claim to an existing
// account. Returns errUserNotFound on miss.
func (s *userStore) getUserByGoogleSub(ctx context.Context, sub string) (*userAccount, error) {
	if strings.TrimSpace(sub) == "" {
		return nil, errUserNotFound
	}
	row := s.db.QueryRowContext(ctx, rb(`SELECT
		id, email, email_lower, password_hash,
		COALESCE(google_sub, ''), name, email_verified, created_at, updated_at
		FROM users WHERE google_sub = ?`), sub)
	return scanUser(row)
}

// linkGoogleSub attaches a Google identifier to an existing account —
// used when a user who signed up with email/password later logs in
// with Google. Fails (errUserExists) if the sub is already linked to
// a different account.
func (s *userStore) linkGoogleSub(ctx context.Context, userID, sub string) error {
	res, err := s.db.ExecContext(ctx, rb(`UPDATE users SET
		google_sub = ?, email_verified = ?, updated_at = ?
		WHERE id = ?`),
		sub, boolForDialect(true), time.Now().UTC(), userID)
	if err != nil {
		if isUniqueViolation(err) {
			return errUserExists
		}
		return fmt.Errorf("link google sub: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errUserNotFound
	}
	return nil
}

type userRow interface {
	Scan(dest ...any) error
}

func scanUser(row userRow) (*userAccount, error) {
	var u userAccount
	var verified any
	err := row.Scan(&u.ID, &u.Email, &u.EmailLower, &u.PasswordHash,
		&u.GoogleSub, &u.Name, &verified, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errUserNotFound
	}
	if err != nil {
		return nil, err
	}
	u.EmailVerified = interpretBool(verified)
	return &u, nil
}

// --- Sessions ----------------------------------------------------------

type userSession struct {
	ID        string
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
	UserAgent string
	IP        string
}

// createSession persists a new session row and returns the opaque
// session ID. The caller is responsible for signing it into a cookie.
func (s *userStore) createSession(ctx context.Context, userID, ua, ip string, ttl time.Duration) (*userSession, error) {
	sess := &userSession{
		ID:        randomID("sess_"),
		UserID:    userID,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
		UserAgent: truncate(ua, 255),
		IP:        truncate(ip, 64),
	}
	_, err := s.db.ExecContext(ctx, rb(`INSERT INTO user_sessions
		(id, user_id, issued_at, expires_at, user_agent, ip)
		VALUES (?, ?, ?, ?, ?, ?)`),
		sess.ID, sess.UserID, sess.IssuedAt, sess.ExpiresAt, sess.UserAgent, sess.IP)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return sess, nil
}

// getSession loads a session for validation. Returns errUserNotFound
// if the session is missing, expired, or revoked — callers should
// treat all three as "not authenticated" (indistinguishable to the
// client).
func (s *userStore) getSession(ctx context.Context, id string) (*userSession, error) {
	row := s.db.QueryRowContext(ctx, rb(`SELECT
		id, user_id, issued_at, expires_at, revoked_at, user_agent, ip
		FROM user_sessions WHERE id = ?`), id)
	var sess userSession
	var revoked sql.NullTime
	err := row.Scan(&sess.ID, &sess.UserID, &sess.IssuedAt, &sess.ExpiresAt, &revoked, &sess.UserAgent, &sess.IP)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if revoked.Valid {
		sess.RevokedAt = &revoked.Time
		return nil, errUserNotFound
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		return nil, errUserNotFound
	}
	return &sess, nil
}

// revokeSession marks a session as revoked (logout).
func (s *userStore) revokeSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, rb(`UPDATE user_sessions
		SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`),
		time.Now().UTC(), id)
	return err
}

// --- Dialect helpers ---------------------------------------------------

// boolForDialect returns 1/0 for SQLite and the bool directly for
// Postgres. The users table uses BOOLEAN on Postgres and INTEGER on
// SQLite, so the same Go bool needs different representations.
func boolForDialect(b bool) any {
	if currentDialect == dialectPostgres {
		return b
	}
	if b {
		return 1
	}
	return 0
}

// interpretBool normalises whatever the driver gives us back (bool,
// int64, []byte) to a native Go bool.
func interpretBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case int64:
		return t != 0
	case int:
		return t != 0
	case []byte:
		return len(t) > 0 && t[0] != '0' && t[0] != 'f' && t[0] != 'F'
	case string:
		return t == "1" || t == "t" || t == "true" || t == "T" || t == "TRUE"
	default:
		return false
	}
}

// isUniqueViolation maps driver-specific unique-constraint errors
// back to a sentinel errUserExists. Both pgx and modernc/sqlite
// return distinctive messages we can match on.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || // sqlite
		strings.Contains(msg, "SQLSTATE 23505") || // postgres (pgx)
		strings.Contains(msg, "duplicate key value")
}


