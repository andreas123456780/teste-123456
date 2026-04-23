package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func newTestAdminCfg(t *testing.T, user, pass string) adminAuthCfg {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return adminAuthCfg{
		username:      user,
		passwordHash:  string(hash),
		sessionSecret: []byte("test-session-secret-32-bytes-xxxx"),
		sessionTTL:    time.Hour,
	}
}

func TestAdminLogin_Success(t *testing.T) {
	cfg := newTestAdminCfg(t, "admin", "hunter2")
	h := handleAdminLogin(cfg)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "hunter2"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	h(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&out)
	if out.Token == "" {
		t.Fatal("expected token in response")
	}
	if user, err := cfg.verifySession(out.Token, time.Now()); err != nil || user != "admin" {
		t.Fatalf("issued token failed verify: user=%q err=%v", user, err)
	}
}

func TestAdminLogin_WrongPassword(t *testing.T) {
	cfg := newTestAdminCfg(t, "admin", "hunter2")
	h := handleAdminLogin(cfg)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	h(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAdminLogin_WrongUser(t *testing.T) {
	cfg := newTestAdminCfg(t, "admin", "hunter2")
	h := handleAdminLogin(cfg)

	body, _ := json.Marshal(map[string]string{"username": "nobody", "password": "hunter2"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	h(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAdminLogin_NotConfigured(t *testing.T) {
	h := handleAdminLogin(adminAuthCfg{})
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "x"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	h(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestAdminAuthFromCfg_SessionAndLegacy(t *testing.T) {
	cfg := newTestAdminCfg(t, "admin", "hunter2")
	cfg.legacyToken = "legacy-token"

	called := false
	h := adminAuthFromCfg(cfg, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// legacy token works
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Admin-Token", "legacy-token")
	h(rr, req)
	if !called || rr.Code != http.StatusOK {
		t.Fatalf("legacy path failed: called=%v code=%d", called, rr.Code)
	}

	// session HMAC works
	session := cfg.makeSession("admin", time.Now())
	called = false
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Admin-Token", session)
	h(rr, req)
	if !called || rr.Code != http.StatusOK {
		t.Fatalf("session path failed: called=%v code=%d", called, rr.Code)
	}

	// garbage → 401
	called = false
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Admin-Token", "nope")
	h(rr, req)
	if called || rr.Code != http.StatusUnauthorized {
		t.Fatalf("garbage path allowed: called=%v code=%d", called, rr.Code)
	}
}

func TestVerifySession_ExpiredRejected(t *testing.T) {
	cfg := newTestAdminCfg(t, "admin", "x")
	cfg.sessionTTL = time.Hour
	// issue a token 2h in the past
	past := time.Now().Add(-2 * time.Hour)
	tok := cfg.makeSession("admin", past)
	if _, err := cfg.verifySession(tok, time.Now()); err == nil {
		t.Fatal("expired token should fail verify")
	}
}

func TestVerifySession_TamperRejected(t *testing.T) {
	cfg := newTestAdminCfg(t, "admin", "x")
	tok := cfg.makeSession("admin", time.Now())
	if len(tok) < 5 {
		t.Fatal("token too short")
	}
	tampered := tok[:len(tok)-2] + "ZZ"
	if _, err := cfg.verifySession(tampered, time.Now()); err == nil {
		t.Fatal("tampered token should fail verify")
	}
}

func TestAdminStats_Empty(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()

	ctx := context.Background()
	stats, err := collectAdminStats(ctx, db)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.Orders.Total != 0 || stats.Revenue.GrossCents != 0 {
		t.Fatalf("empty DB should yield zeros, got %+v", stats)
	}
}

func TestAdminStats_CountsPaidOnly(t *testing.T) {
	store, db, cleanup := newTestStore(t)
	defer cleanup()

	o1 := putTestOrder(t, store, "o-pending", "a@b.com", "pix", 10000, 1000)
	_ = o1
	o2 := putTestOrder(t, store, "o-paid", "a@b.com", "card", 20000, 2000)
	if err := store.setStatus(context.Background(), o2.ID, "paid"); err != nil {
		t.Fatalf("setStatus: %v", err)
	}

	stats, err := collectAdminStats(context.Background(), db)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.Orders.Total != 2 || stats.Orders.Paid != 1 || stats.Orders.PendingPayment != 1 {
		t.Fatalf("bad breakdown: %+v", stats.Orders)
	}
	if stats.Revenue.GrossCents != 20000 || stats.Revenue.PaidOrderCount != 1 {
		t.Fatalf("bad revenue: %+v", stats.Revenue)
	}
	if stats.Revenue.AvgTicketCents != 20000 {
		t.Fatalf("avg ticket want 20000 got %d", stats.Revenue.AvgTicketCents)
	}
}
