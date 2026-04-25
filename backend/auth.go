package main

// Customer-facing authentication.
//
// Two signup paths — email/password and Google OAuth — both converge
// on a single users row and a single session cookie. The cookie is an
// HMAC-signed envelope over the session ID issued by userStore.
//
// Cookie format: nast_session="sessV1:<sid>:<sig>".
// The signature binds the session id to AUTH_SESSION_SECRET so that
// leaked session ids from an unrelated system can never be replayed.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName  = "nast_session"
	sessionCookieMaxAge = 30 * 24 * time.Hour // 30 days
	// bcryptCost follows the default tuned for 2024 hardware. Matching
	// the admin_login.go cost keeps hash comparisons uniform.
	bcryptCost = 12
)

// authConfig wraps the shared secrets the handlers and middleware
// need. The zero value disables the entire customer-auth surface so
// the backend remains deployable before the feature is turned on in
// prod.
type authConfig struct {
	sessionSecret []byte
	// cookieSecure gates the `Secure` cookie attribute. True in
	// production (HTTPS-only), false for local dev over HTTP.
	cookieSecure bool
	// cookieDomain is optional; when set, scopes the cookie to the
	// apex domain so api.nast.com.br and nast.com.br share sessions.
	cookieDomain string
}

func loadAuthConfig() authConfig {
	cfg := authConfig{}
	if s := strings.TrimSpace(os.Getenv("AUTH_SESSION_SECRET")); s != "" {
		cfg.sessionSecret = []byte(s)
	}
	cfg.cookieSecure = strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_COOKIE_SECURE")), "true")
	// Default to secure on Vercel (always HTTPS) unless explicitly disabled.
	if !cfg.cookieSecure && os.Getenv("VERCEL") != "" {
		cfg.cookieSecure = true
	}
	cfg.cookieDomain = strings.TrimSpace(os.Getenv("AUTH_COOKIE_DOMAIN"))
	return cfg
}

// enabled returns true when the feature is fully configured. If
// false, handlers return 503 so callers can tell "auth is off" apart
// from "wrong credentials".
func (c authConfig) enabled() bool {
	return len(c.sessionSecret) >= 16
}

// --- Cookie-level signing ---------------------------------------------

// makeCookie signs the opaque session id with HMAC and returns the
// cookie-ready string. Keeping the signature separate from the session
// id means a leaked DB row cannot be turned into a forged cookie
// without also leaking AUTH_SESSION_SECRET.
func (c authConfig) makeCookie(sessionID string) string {
	mac := hmac.New(sha256.New, c.sessionSecret)
	mac.Write([]byte("sessV1:" + sessionID))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return "sessV1:" + sessionID + ":" + sig
}

var errCookieInvalid = errors.New("invalid session cookie")

// parseCookie validates the signature and returns the session id.
func (c authConfig) parseCookie(raw string) (string, error) {
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) != 3 || parts[0] != "sessV1" {
		return "", errCookieInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errCookieInvalid
	}
	mac := hmac.New(sha256.New, c.sessionSecret)
	mac.Write([]byte("sessV1:" + parts[1]))
	if !hmac.Equal(mac.Sum(nil), sig) {
		return "", errCookieInvalid
	}
	return parts[1], nil
}

// sameSiteForCfg picks the appropriate SameSite attribute. In
// production (cookieSecure=true) the SPA fetches the backend on a
// separate origin, so the cookie needs SameSite=None to travel with
// the request — browsers require Secure for None. In local dev
// (HTTP, cookieSecure=false) we fall back to Lax so the cookie is
// still accepted without HTTPS.
func sameSiteForCfg(c authConfig) http.SameSite {
	if c.cookieSecure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func (c authConfig) setSessionCookie(w http.ResponseWriter, sessionID string) {
	ck := &http.Cookie{
		Name:     sessionCookieName,
		Value:    c.makeCookie(sessionID),
		Path:     "/",
		Expires:  time.Now().Add(sessionCookieMaxAge),
		MaxAge:   int(sessionCookieMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: sameSiteForCfg(c),
	}
	if c.cookieDomain != "" {
		ck.Domain = c.cookieDomain
	}
	http.SetCookie(w, ck)
}

func (c authConfig) clearSessionCookie(w http.ResponseWriter) {
	ck := &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: sameSiteForCfg(c),
	}
	if c.cookieDomain != "" {
		ck.Domain = c.cookieDomain
	}
	http.SetCookie(w, ck)
}

// --- Middleware -------------------------------------------------------

type ctxKey string

const ctxKeyUser ctxKey = "auth.user"

// currentUser returns the authenticated user attached to the request
// context, or nil when the request is anonymous. Handlers that
// require auth should use requireAuth — currentUser is for handlers
// that branch on the user being present (e.g., checkout pre-fill).
func currentUser(r *http.Request) *userAccount {
	v := r.Context().Value(ctxKeyUser)
	if v == nil {
		return nil
	}
	u, _ := v.(*userAccount)
	return u
}

// loadCurrentUser is middleware that extracts and validates the
// session cookie, attaching the matching userAccount to the request
// context. Unauthenticated requests pass through untouched — the
// handler decides whether that's allowed.
func loadCurrentUser(cfg authConfig, users *userStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.enabled() || users == nil {
				next.ServeHTTP(w, r)
				return
			}
			ck, err := r.Cookie(sessionCookieName)
			if err != nil || ck.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			sid, err := cfg.parseCookie(ck.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			sess, err := users.getSession(ctx, sid)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			u, err := users.getUserByID(ctx, sess.UserID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			rr := r.WithContext(context.WithValue(r.Context(), ctxKeyUser, u))
			next.ServeHTTP(w, rr)
		})
	}
}

// requireAuth rejects anonymous requests with 401.
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// --- Signup / Login / Logout ------------------------------------------

type signupRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authMeResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	EmailVerified bool   `json:"emailVerified"`
	HasPassword   bool   `json:"hasPassword"`
	HasGoogle     bool   `json:"hasGoogle"`
}

func toAuthMe(u *userAccount) authMeResponse {
	return authMeResponse{
		ID:            u.ID,
		Email:         u.Email,
		Name:          u.Name,
		EmailVerified: u.EmailVerified,
		HasPassword:   u.PasswordHash != "",
		HasGoogle:     u.GoogleSub != "",
	}
}

// validatePassword enforces a baseline policy. Deliberately modest:
// length + variety, no silly rules that push users to reuse.
func validatePassword(p string) error {
	if len(p) < 8 {
		return errors.New("senha deve ter ao menos 8 caracteres")
	}
	if len(p) > 256 {
		return errors.New("senha muito longa")
	}
	hasLetter, hasNumber := false, false
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasNumber = true
		}
	}
	if !hasLetter || !hasNumber {
		return errors.New("senha deve ter letras e números")
	}
	return nil
}

// handleSignup creates a new email/password user, auto-logs-in and
// returns the profile. Rejects duplicate emails with 409.
func handleSignup(cfg authConfig, users *userStore, orders *orderStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !cfg.enabled() || users == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
			return
		}
		var req signupRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		email := strings.TrimSpace(req.Email)
		name := strings.TrimSpace(req.Name)
		if !validateEmail(email) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email inválido"})
			return
		}
		if len(name) < 2 || len(name) > 120 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nome inválido"})
			return
		}
		if err := validatePassword(req.Password); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
		if err != nil {
			log.Printf("auth.signup: hash: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		u := &userAccount{
			Email:        email,
			Name:         name,
			PasswordHash: string(hash),
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := users.createUser(ctx, u); err != nil {
			if errors.Is(err, errUserExists) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "email já cadastrado"})
				return
			}
			log.Printf("auth.signup: create: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if err := issueSession(ctx, cfg, users, u, w, r); err != nil {
			log.Printf("auth.signup: session: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		linkLegacyOrders(ctx, orders, u)
		writeJSON(w, http.StatusCreated, toAuthMe(u))
	}
}

// handleLogin verifies email+password and issues a session cookie.
// Always responds 401 on mismatch — never distinguishes "no such
// email" from "wrong password" to defeat enumeration.
func handleLogin(cfg authConfig, users *userStore, orders *orderStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !cfg.enabled() || users == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
			return
		}
		var req loginRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		u, err := users.getUserByEmail(ctx, req.Email)
		if err != nil || u.PasswordHash == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "credenciais inválidas"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "credenciais inválidas"})
			return
		}
		if err := issueSession(ctx, cfg, users, u, w, r); err != nil {
			log.Printf("auth.login: session: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		linkLegacyOrders(ctx, orders, u)
		writeJSON(w, http.StatusOK, toAuthMe(u))
	}
}

// handleLogout revokes the current session (so the cookie can't be
// replayed) and clears the cookie on the client.
func handleLogout(cfg authConfig, users *userStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if cfg.enabled() && users != nil {
			if ck, err := r.Cookie(sessionCookieName); err == nil {
				if sid, err := cfg.parseCookie(ck.Value); err == nil {
					ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
					defer cancel()
					_ = users.revokeSession(ctx, sid)
				}
			}
		}
		cfg.clearSessionCookie(w)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleMe returns the current user, or 401 when unauthenticated.
func handleMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, toAuthMe(u))
}

// linkLegacyOrders is a best-effort retroactive attachment of a
// freshly-authenticated user to orders placed before they had an
// account (matched by email_lower). Errors are logged and swallowed
// because the /minha-conta endpoint still falls back to email-match
// at read time — linking is a performance/consistency optimization,
// not a correctness requirement.
func linkLegacyOrders(ctx context.Context, orders *orderStore, u *userAccount) {
	if orders == nil || u == nil {
		return
	}
	if err := orders.linkOrdersByEmail(ctx, u.ID, strings.ToLower(u.Email)); err != nil {
		log.Printf("auth: link legacy orders for %s: %v", u.ID, err)
	}
}

// issueSession creates a DB row + writes the signed cookie.
func issueSession(ctx context.Context, cfg authConfig, users *userStore, u *userAccount, w http.ResponseWriter, r *http.Request) error {
	sess, err := users.createSession(ctx, u.ID, r.UserAgent(), authClientIP(r), sessionCookieMaxAge)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	cfg.setSessionCookie(w, sess.ID)
	return nil
}

// authClientIP is a simple XFF-aware extractor used only to label
// session rows for audit. Not security-sensitive — the real
// anti-abuse lives in the shared rate limiter.
func authClientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		parts := strings.SplitN(xf, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}
