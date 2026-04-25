package main

// Tests for the customer auth surface: signup, login, logout, me,
// session cookie signing, and Google OAuth callback.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestAuthCfg() authConfig {
	return authConfig{
		sessionSecret: bytes.Repeat([]byte("s"), 32),
		cookieSecure:  false,
	}
}

func newAuthMux(t *testing.T) (*http.ServeMux, *userStore, authConfig, func()) {
	t.Helper()
	store, db, cleanup := newTestStore(t)
	users := newUserStore(db)
	cfg := newTestAuthCfg()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/signup", handleSignup(cfg, users, store))
	mux.HandleFunc("/api/auth/login", handleLogin(cfg, users, store))
	mux.HandleFunc("/api/auth/logout", handleLogout(cfg, users))
	mux.Handle("/api/auth/me", loadCurrentUser(cfg, users)(http.HandlerFunc(handleMe)))
	return mux, users, cfg, cleanup
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func sessionCookieFrom(t *testing.T, rr *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == sessionCookieName && ck.Value != "" {
			return ck
		}
	}
	t.Fatalf("no session cookie in response; got %v", rr.Result().Cookies())
	return nil
}

func TestSignup_HappyPath(t *testing.T) {
	mux, _, _, cleanup := newAuthMux(t)
	defer cleanup()

	rr := doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "Andreas Teste", Email: "a@b.com", Password: "hunter2pass",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var me authMeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Email != "a@b.com" || me.Name != "Andreas Teste" {
		t.Errorf("unexpected /me: %+v", me)
	}
	if !me.HasPassword || me.HasGoogle {
		t.Errorf("flags: want password=true google=false, got %+v", me)
	}
	if ck := sessionCookieFrom(t, rr); ck == nil {
		t.Fatal("no cookie")
	}
}

func TestSignup_DuplicateEmail(t *testing.T) {
	mux, _, _, cleanup := newAuthMux(t)
	defer cleanup()

	// First signup wins.
	rr := doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "Andreas", Email: "dup@b.com", Password: "hunter2pass",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("first signup: want 201, got %d", rr.Code)
	}
	// Second attempt with a casing variation must 409.
	rr = doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "Other", Email: "DUP@B.com", Password: "hunter2pass",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("dup signup: want 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSignup_WeakPassword(t *testing.T) {
	mux, _, _, cleanup := newAuthMux(t)
	defer cleanup()

	cases := []string{"", "short", "12345678", "abcdefgh"}
	for _, p := range cases {
		rr := doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
			Name: "Nome Teste", Email: "weak" + p + "@b.com", Password: p,
		})
		if rr.Code != http.StatusBadRequest {
			t.Errorf("password %q: want 400, got %d", p, rr.Code)
		}
	}
}

func TestLogin_HappyPath(t *testing.T) {
	mux, _, _, cleanup := newAuthMux(t)
	defer cleanup()

	doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "Andreas Teste", Email: "login@b.com", Password: "hunter2pass",
	})
	rr := doJSON(t, mux, "POST", "/api/auth/login", loginRequest{
		Email: "LOGIN@b.com", Password: "hunter2pass",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	sessionCookieFrom(t, rr)
}

func TestLogin_WrongPassword(t *testing.T) {
	mux, _, _, cleanup := newAuthMux(t)
	defer cleanup()

	doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "Andreas Teste", Email: "wrong@b.com", Password: "hunter2pass",
	})
	rr := doJSON(t, mux, "POST", "/api/auth/login", loginRequest{
		Email: "wrong@b.com", Password: "different-pass",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	mux, _, _, cleanup := newAuthMux(t)
	defer cleanup()

	rr := doJSON(t, mux, "POST", "/api/auth/login", loginRequest{
		Email: "nobody@b.com", Password: "hunter2pass",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestMe_WithCookie(t *testing.T) {
	mux, _, _, cleanup := newAuthMux(t)
	defer cleanup()

	rr := doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "Andreas Teste", Email: "me@b.com", Password: "hunter2pass",
	})
	ck := sessionCookieFrom(t, rr)

	rr = doJSON(t, mux, "GET", "/api/auth/me", nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var me authMeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Email != "me@b.com" {
		t.Errorf("unexpected /me: %+v", me)
	}
}

func TestMe_NoCookie(t *testing.T) {
	mux, _, _, cleanup := newAuthMux(t)
	defer cleanup()

	rr := doJSON(t, mux, "GET", "/api/auth/me", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestLogout_RevokesSession(t *testing.T) {
	mux, users, _, cleanup := newAuthMux(t)
	defer cleanup()

	rr := doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "Andreas Teste", Email: "logout@b.com", Password: "hunter2pass",
	})
	ck := sessionCookieFrom(t, rr)

	rr = doJSON(t, mux, "POST", "/api/auth/logout", nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("logout: want 200, got %d", rr.Code)
	}

	// /me must now be 401 even with the old cookie.
	rr = doJSON(t, mux, "GET", "/api/auth/me", nil, ck)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout /me: want 401, got %d", rr.Code)
	}

	// Internally the session row must be revoked.
	cfg := newTestAuthCfg()
	sid, err := cfg.parseCookie(ck.Value)
	if err != nil {
		t.Fatalf("parse cookie: %v", err)
	}
	if _, err := users.getSession(context.Background(), sid); !errors.Is(err, errUserNotFound) {
		t.Errorf("want errUserNotFound on revoked session, got %v", err)
	}
}

func TestCookieSigning_RejectsForged(t *testing.T) {
	cfg := newTestAuthCfg()
	cookie := cfg.makeCookie("sess_abc")
	if _, err := cfg.parseCookie(cookie); err != nil {
		t.Fatalf("valid cookie rejected: %v", err)
	}
	// Swap the signature with garbage.
	parts := strings.Split(cookie, ":")
	parts[2] = "AAAAAAAA"
	forged := strings.Join(parts, ":")
	if _, err := cfg.parseCookie(forged); err == nil {
		t.Fatal("forged cookie accepted")
	}
}

func TestFindOrCreateGoogleUser_NewUser(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	users := newUserStore(db)

	u, err := findOrCreateGoogleUser(context.Background(), users, &googleClaims{
		Sub: "google-123", Email: "newgoogle@b.com", Name: "Novo", EmailVerified: true,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.Email != "newgoogle@b.com" || u.GoogleSub != "google-123" || !u.EmailVerified {
		t.Errorf("unexpected user: %+v", u)
	}
}

func TestFindOrCreateGoogleUser_LinksExistingByEmail(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	users := newUserStore(db)

	// Seed with an email/password account first.
	existing := &userAccount{Email: "link@b.com", Name: "Pre", PasswordHash: "xxx"}
	if err := users.createUser(context.Background(), existing); err != nil {
		t.Fatal(err)
	}

	// Google login with the same email must link, not create.
	u, err := findOrCreateGoogleUser(context.Background(), users, &googleClaims{
		Sub: "google-456", Email: "link@b.com", Name: "Google Name",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.ID != existing.ID {
		t.Errorf("expected to link same user, got %s vs %s", u.ID, existing.ID)
	}
	if u.GoogleSub != "google-456" {
		t.Errorf("google sub not linked: %+v", u)
	}
}

func TestFindOrCreateGoogleUser_RejectsSubCollision(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	users := newUserStore(db)

	// Two different users, same email, different google subs should
	// not happen — but if an attacker sent a claim for an email that
	// already exists linked to a *different* google sub, we refuse.
	u1 := &userAccount{Email: "a@b.com", Name: "A", GoogleSub: "sub-1"}
	if err := users.createUser(context.Background(), u1); err != nil {
		t.Fatal(err)
	}
	_, err := findOrCreateGoogleUser(context.Background(), users, &googleClaims{
		Sub: "sub-2", Email: "a@b.com", Name: "Impostor",
	})
	if !errors.Is(err, errUserExists) {
		t.Fatalf("want errUserExists on sub collision, got %v", err)
	}
}

func TestAuthDisabled_Returns503(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	users := newUserStore(db)
	cfg := authConfig{} // zero-value = disabled

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/signup", handleSignup(cfg, users, nil))
	mux.HandleFunc("/api/auth/login", handleLogin(cfg, users, nil))

	for _, path := range []string{"/api/auth/signup", "/api/auth/login"} {
		rr := doJSON(t, mux, "POST", path, map[string]string{
			"email": "x@b.com", "password": "hunter2pass", "name": "X Y",
		})
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: want 503, got %d", path, rr.Code)
		}
	}
}
