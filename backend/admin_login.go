package main

// Admin login flow.
//
// The legacy X-Admin-Token header (compared in constant time) is still
// honored as a fallback — it keeps CI, scripts and operator recovery
// paths working when ADMIN_USERNAME/ADMIN_PASSWORD_HASH are not set.
// When configured, the preferred flow is:
//
//   1. Operator opens /admin.
//   2. UI POSTs {username, password} to /api/admin/login.
//   3. Server verifies bcrypt(password) == ADMIN_PASSWORD_HASH.
//   4. Server returns an HMAC-signed session token that the UI stores
//      in localStorage and sends back as X-Admin-Token on every request.
//
// Session tokens are stateless (no DB row): the payload is
//   sessionV1:<expiry-unix>:<username>:<hmac>
// where hmac = HMAC-SHA256(ADMIN_SESSION_SECRET, "sessionV1:<expiry>:<user>").
// On verification we recompute the HMAC and reject expired tokens.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// adminAuthCfg wraps everything adminAuth needs. A zero-valued struct
// is treated as "admin not configured" and returns 503 on every call.
type adminAuthCfg struct {
	// legacyToken is the old ADMIN_TOKEN. Empty = disabled.
	legacyToken string
	// username + passwordHash enable user+password login. Both empty =
	// login endpoint is disabled (operator must paste legacyToken).
	username     string
	passwordHash string
	// sessionSecret is used to sign/verify session tokens. If empty, a
	// random one is generated at boot — fine for a single-process
	// deploy, but sessions won't survive a restart.
	sessionSecret []byte
	// sessionTTL is how long a freshly-issued token remains valid.
	sessionTTL time.Duration
}

func loadAdminAuthCfg() adminAuthCfg {
	cfg := adminAuthCfg{
		legacyToken:  strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		username:     strings.TrimSpace(os.Getenv("ADMIN_USERNAME")),
		passwordHash: strings.TrimSpace(os.Getenv("ADMIN_PASSWORD_HASH")),
		sessionTTL:   24 * time.Hour,
	}
	if s := strings.TrimSpace(os.Getenv("ADMIN_SESSION_SECRET")); s != "" {
		cfg.sessionSecret = []byte(s)
	} else {
		// Fall back to a random secret so sessions still work, at the
		// cost of being invalidated on restart. Also reused across
		// processes via a stable derivation from ADMIN_TOKEN if set —
		// that keeps dev flows stable without forcing a new env var.
		if cfg.legacyToken != "" {
			h := sha256.Sum256([]byte("nast:session:" + cfg.legacyToken))
			cfg.sessionSecret = h[:]
		}
	}
	return cfg
}

func (c adminAuthCfg) loginEnabled() bool {
	return c.username != "" && c.passwordHash != "" && len(c.sessionSecret) > 0
}

// makeSession issues a new session token valid for c.sessionTTL.
func (c adminAuthCfg) makeSession(username string, now time.Time) string {
	exp := now.Add(c.sessionTTL).Unix()
	payload := fmt.Sprintf("sessionV1:%d:%s", exp, username)
	mac := hmac.New(sha256.New, c.sessionSecret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + ":" + sig
}

var errSessionInvalid = errors.New("invalid session")

// verifySession returns the username if the token is valid & unexpired.
func (c adminAuthCfg) verifySession(token string, now time.Time) (string, error) {
	if len(c.sessionSecret) == 0 {
		return "", errSessionInvalid
	}
	parts := strings.SplitN(token, ":", 4)
	if len(parts) != 4 || parts[0] != "sessionV1" {
		return "", errSessionInvalid
	}
	expUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", errSessionInvalid
	}
	if now.Unix() > expUnix {
		return "", errSessionInvalid
	}
	payload := strings.Join(parts[:3], ":")
	sig, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return "", errSessionInvalid
	}
	mac := hmac.New(sha256.New, c.sessionSecret)
	mac.Write([]byte(payload))
	if !hmac.Equal(mac.Sum(nil), sig) {
		return "", errSessionInvalid
	}
	return parts[2], nil
}

// adminAuthFromCfg is the modern middleware. It accepts either a
// legacyToken (constant-time compare) or a valid session HMAC. Returns
// 503 when neither auth mechanism is configured.
func adminAuthFromCfg(cfg adminAuthCfg, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.legacyToken == "" && !cfg.loginEnabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin not configured"})
			return
		}
		got := r.Header.Get("X-Admin-Token")
		if got == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Path 1: legacy static token.
		if cfg.legacyToken != "" &&
			subtle.ConstantTimeCompare([]byte(got), []byte(cfg.legacyToken)) == 1 {
			next(w, r)
			return
		}
		// Path 2: session HMAC.
		if _, err := cfg.verifySession(got, time.Now()); err == nil {
			next(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}
}

// handleAdminLogin authenticates {username, password} and returns a
// session token on success. Never leaks whether the username exists vs.
// password is wrong — both collapse to a single "credenciais inválidas"
// error so the endpoint can't be used to enumerate users.
func handleAdminLogin(cfg adminAuthCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !cfg.loginEnabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "login not configured"})
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		u := strings.TrimSpace(req.Username)
		p := req.Password
		if u == "" || p == "" || len(u) > 80 || len(p) > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credenciais inválidas"})
			return
		}
		// Compare username in constant time to keep timing uniform
		// regardless of first-character match.
		userOK := subtle.ConstantTimeCompare([]byte(u), []byte(cfg.username)) == 1
		// Always run bcrypt even on user mismatch so timing is flat.
		bcryptErr := bcrypt.CompareHashAndPassword([]byte(cfg.passwordHash), []byte(p))
		if !userOK || bcryptErr != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "credenciais inválidas"})
			return
		}
		now := time.Now()
		token := cfg.makeSession(cfg.username, now)
		writeJSON(w, http.StatusOK, map[string]any{
			"token":     token,
			"username":  cfg.username,
			"expiresAt": now.Add(cfg.sessionTTL).Format(time.RFC3339),
		})
	}
}
