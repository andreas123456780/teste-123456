package main

// Google Sign-In (OAuth 2.0 / OpenID Connect).
//
// Flow:
//   1. Browser hits /api/auth/google/start
//        → we generate a fresh `state` nonce, stash it in a short-lived
//          cookie, and 302 to https://accounts.google.com/o/oauth2/v2/auth
//   2. Google redirects to /api/auth/google/callback?code=…&state=…
//        → we verify the `state` cookie, exchange the code for an
//          id_token, parse the email+sub out of it, then either log
//          in the existing user or create a new account.
//
// We only rely on the id_token — no access token is stored — because
// we just need email + sub. This keeps the surface area small and
// avoids having to ever re-authenticate against Google APIs later.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type googleConfig struct {
	ClientID     string
	ClientSecret string
	// RedirectURL must match one of the "Authorized redirect URIs"
	// configured in the Google Cloud Console OAuth client.
	RedirectURL string
	// PostLoginURL is where to send the browser after a successful
	// login (typically the frontend origin + /minha-conta).
	PostLoginURL string
	// FailureURL is where to redirect on error. Query string includes
	// `?auth_error=<code>` so the frontend can display a message.
	FailureURL string
}

func loadGoogleConfig() googleConfig {
	return googleConfig{
		ClientID:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
		RedirectURL:  strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL")),
		PostLoginURL: defaultStr(os.Getenv("GOOGLE_POST_LOGIN_URL"), os.Getenv("APP_URL")),
		FailureURL:   defaultStr(os.Getenv("GOOGLE_FAILURE_URL"), os.Getenv("APP_URL")+"/login"),
	}
}

func (c googleConfig) enabled() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURL != ""
}

const (
	googleStateCookie = "nast_oauth_state"
	googleStateTTL    = 10 * time.Minute
)

// handleGoogleStart begins the OAuth dance by redirecting to Google.
func handleGoogleStart(authCfg authConfig, gcfg googleConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !gcfg.enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "google oauth not configured"})
			return
		}
		if !authCfg.enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
			return
		}
		state, err := randomState()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		setStateCookie(w, authCfg, state)

		q := url.Values{}
		q.Set("client_id", gcfg.ClientID)
		q.Set("redirect_uri", gcfg.RedirectURL)
		q.Set("response_type", "code")
		q.Set("scope", "openid email profile")
		q.Set("state", state)
		// "select_account" forces the chooser every time — helpful when
		// someone is logged into multiple Google accounts.
		q.Set("prompt", "select_account")
		http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+q.Encode(), http.StatusFound)
	}
}

// handleGoogleCallback exchanges the auth code, matches/creates the
// user, and redirects back to the frontend with a session cookie set.
func handleGoogleCallback(authCfg authConfig, gcfg googleConfig, users *userStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !gcfg.enabled() || !authCfg.enabled() || users == nil {
			redirectFailure(w, r, gcfg, "unavailable")
			return
		}
		if errCode := r.URL.Query().Get("error"); errCode != "" {
			redirectFailure(w, r, gcfg, errCode)
			return
		}
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		if state == "" || code == "" {
			redirectFailure(w, r, gcfg, "missing_params")
			return
		}
		if !verifyStateCookie(r, authCfg, state) {
			redirectFailure(w, r, gcfg, "state_mismatch")
			return
		}
		// One-shot; clear the state cookie so it cannot be replayed.
		clearStateCookie(w, authCfg)

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		claims, err := exchangeGoogleCode(ctx, gcfg, code)
		if err != nil {
			log.Printf("auth.google.callback: exchange: %v", err)
			redirectFailure(w, r, gcfg, "exchange_failed")
			return
		}
		if claims.Email == "" || claims.Sub == "" {
			redirectFailure(w, r, gcfg, "missing_claims")
			return
		}

		user, err := findOrCreateGoogleUser(ctx, users, claims)
		if err != nil {
			if errors.Is(err, errUserExists) {
				// The email belongs to a different Google sub. Refuse
				// rather than merge blindly.
				redirectFailure(w, r, gcfg, "email_collision")
				return
			}
			log.Printf("auth.google.callback: upsert: %v", err)
			redirectFailure(w, r, gcfg, "internal")
			return
		}
		if err := issueSession(ctx, authCfg, users, user, w, r); err != nil {
			log.Printf("auth.google.callback: session: %v", err)
			redirectFailure(w, r, gcfg, "internal")
			return
		}
		http.Redirect(w, r, gcfg.PostLoginURL, http.StatusFound)
	}
}

// --- Helpers ----------------------------------------------------------

// googleClaims is the subset of the ID token we care about. See
// https://developers.google.com/identity/openid-connect/openid-connect#id_token.
type googleClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// exchangeGoogleCode swaps the authorization code for an ID token and
// returns the parsed claims. We trust the token because it was fetched
// over TLS directly from Google's token endpoint in a server-to-server
// call; there's no middle-box, so we skip JWT signature verification.
var exchangeGoogleCode = func(ctx context.Context, cfg googleConfig, code string) (*googleClaims, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("redirect_uri", cfg.RedirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google token: %d %s", resp.StatusCode, truncate(string(body), 200))
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("google token parse: %w", err)
	}
	return parseGoogleIDToken(tok.IDToken)
}

// parseGoogleIDToken base64-decodes the JWT payload — no signature
// verification because the token came from a TLS response body we
// trust (see exchangeGoogleCode).
func parseGoogleIDToken(idToken string) (*googleClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed id_token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("id_token payload decode: %w", err)
	}
	var c googleClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("id_token payload parse: %w", err)
	}
	return &c, nil
}

// findOrCreateGoogleUser resolves the Google claims to a local user
// account, handling all three cases:
//   - existing user by google_sub → return it
//   - existing user by email only → link the google_sub, return it
//   - neither → create a brand-new user
func findOrCreateGoogleUser(ctx context.Context, users *userStore, c *googleClaims) (*userAccount, error) {
	if u, err := users.getUserByGoogleSub(ctx, c.Sub); err == nil {
		return u, nil
	}
	existing, err := users.getUserByEmail(ctx, c.Email)
	if err == nil {
		if existing.GoogleSub != "" && existing.GoogleSub != c.Sub {
			return nil, errUserExists
		}
		if existing.GoogleSub == "" {
			if err := users.linkGoogleSub(ctx, existing.ID, c.Sub); err != nil {
				return nil, err
			}
			existing.GoogleSub = c.Sub
			existing.EmailVerified = true
		}
		return existing, nil
	}
	u := &userAccount{
		Email:         c.Email,
		Name:          c.Name,
		GoogleSub:     c.Sub,
		EmailVerified: c.EmailVerified,
	}
	if err := users.createUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// setStateCookie writes an HMAC-signed state value so we can verify
// it on callback without keeping server-side state. The same
// AUTH_SESSION_SECRET is reused.
func setStateCookie(w http.ResponseWriter, cfg authConfig, state string) {
	mac := hmac.New(sha256.New, cfg.sessionSecret)
	mac.Write([]byte("oauthState:" + state))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{
		Name:     googleStateCookie,
		Value:    state + ":" + sig,
		Path:     "/api/auth/google/",
		Expires:  time.Now().Add(googleStateTTL),
		MaxAge:   int(googleStateTTL.Seconds()),
		HttpOnly: true,
		Secure:   cfg.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearStateCookie(w http.ResponseWriter, cfg authConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleStateCookie,
		Value:    "",
		Path:     "/api/auth/google/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func verifyStateCookie(r *http.Request, cfg authConfig, state string) bool {
	ck, err := r.Cookie(googleStateCookie)
	if err != nil {
		return false
	}
	parts := strings.SplitN(ck.Value, ":", 2)
	if len(parts) != 2 || parts[0] != state {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, cfg.sessionSecret)
	mac.Write([]byte("oauthState:" + state))
	return hmac.Equal(mac.Sum(nil), sig)
}

func redirectFailure(w http.ResponseWriter, r *http.Request, cfg googleConfig, reason string) {
	dst := cfg.FailureURL
	if dst == "" {
		dst = "/"
	}
	sep := "?"
	if strings.Contains(dst, "?") {
		sep = "&"
	}
	dst += sep + "auth_error=" + url.QueryEscape(reason)
	http.Redirect(w, r, dst, http.StatusFound)
}

func defaultStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
