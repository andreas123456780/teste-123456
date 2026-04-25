package main

// Tests for /api/account/orders — the /minha-conta backing endpoint.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newAccountMux(t *testing.T) (*http.ServeMux, *orderStore, *userStore, authConfig, func()) {
	t.Helper()
	orders, db, cleanup := newTestStore(t)
	users := newUserStore(db)
	cfg := newTestAuthCfg()
	mux := http.NewServeMux()
	tokenKey := bytes.Repeat([]byte("k"), 32)
	mux.HandleFunc("/api/auth/signup", handleSignup(cfg, users, orders))
	mux.HandleFunc("/api/auth/login", handleLogin(cfg, users, orders))
	mux.Handle("/api/account/orders",
		loadCurrentUser(cfg, users)(http.HandlerFunc(handleMyOrders(cfg, orders, tokenKey))))
	return mux, orders, users, cfg, cleanup
}

func TestMyOrders_Unauthenticated(t *testing.T) {
	mux, _, _, _, cleanup := newAccountMux(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodGet, "/api/account/orders", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestMyOrders_AuthDisabled503(t *testing.T) {
	orders, db, cleanup := newTestStore(t)
	defer cleanup()
	users := newUserStore(db)
	cfg := authConfig{}
	tokenKey := bytes.Repeat([]byte("k"), 32)
	h := handleMyOrders(cfg, orders, tokenKey)
	req := httptest.NewRequest(http.MethodGet, "/api/account/orders", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
	_ = users
}

func TestMyOrders_ReturnsUsersOrdersIncludingLegacy(t *testing.T) {
	mux, orders, _, _, cleanup := newAccountMux(t)
	defer cleanup()

	// Legacy order created before the user had an account. user_id
	// is empty; email matches the future signup.
	legacy := &pendingOrder{
		ID:            "ord_legacy",
		Name:          "Andreas Teste",
		Email:         "Andreas@B.com",
		Address:       "Rua A 1",
		Zip:           "01000-000",
		PaymentMethod: "card",
		Status:        "shipped",
		TotalCents:    10000,
		AmountCents:   10000,
		TrackingCode:  "BR123",
		CreatedAt:     time.Now().Add(-24 * time.Hour).UTC(),
		Items: []orderItem{
			{ProductID: "p1", ProductName: "Tee", Quantity: 1, UnitPriceCents: 10000},
		},
	}
	if err := orders.create(context.Background(), legacy); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	// Sign up with the same email — linkLegacyOrders should attach
	// legacy.user_id to the freshly-created user row.
	rr := doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "Andreas Teste", Email: "andreas@b.com", Password: "hunter2pass",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("signup: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	sessCk := sessionCookieFrom(t, rr)

	// Ask for the user's orders.
	req := httptest.NewRequest(http.MethodGet, "/api/account/orders", nil)
	req.AddCookie(sessCk)
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET /api/account/orders: want 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var body struct {
		Orders []myOrder `json:"orders"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Orders) != 1 {
		t.Fatalf("want 1 order, got %d", len(body.Orders))
	}
	got := body.Orders[0]
	if got.ID != "ord_legacy" {
		t.Errorf("wrong order: %s", got.ID)
	}
	if got.TrackingCode != "BR123" {
		t.Errorf("tracking missing: %+v", got)
	}
	if got.OrderToken == "" {
		t.Errorf("expected order token to be generated")
	}
	if len(got.Items) != 1 || got.Items[0].ProductName != "Tee" {
		t.Errorf("items projection wrong: %+v", got.Items)
	}
}

func TestMyOrders_ExcludesPendingPayment(t *testing.T) {
	mux, orders, _, _, cleanup := newAccountMux(t)
	defer cleanup()

	rr := doJSON(t, mux, "POST", "/api/auth/signup", signupRequest{
		Name: "User Teste", Email: "pend@b.com", Password: "hunter2pass",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("signup: want 201, got %d", rr.Code)
	}
	sessCk := sessionCookieFrom(t, rr)

	// One pending_payment order + one paid. Only paid should show.
	pending := &pendingOrder{
		ID: "ord_pend", Name: "U", Email: "pend@b.com", Address: "x", Zip: "1",
		PaymentMethod: "card", Status: "pending_payment",
		TotalCents: 100, AmountCents: 100,
		CreatedAt: time.Now().UTC(),
	}
	paid := &pendingOrder{
		ID: "ord_paid", Name: "U", Email: "pend@b.com", Address: "x", Zip: "1",
		PaymentMethod: "card", Status: "paid",
		TotalCents: 200, AmountCents: 200,
		CreatedAt: time.Now().UTC(),
	}
	for _, o := range []*pendingOrder{pending, paid} {
		if err := orders.create(context.Background(), o); err != nil {
			t.Fatalf("seed %s: %v", o.ID, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/account/orders", nil)
	req.AddCookie(sessCk)
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "ord_paid") {
		t.Errorf("missing ord_paid: %s", rr2.Body.String())
	}
	if strings.Contains(rr2.Body.String(), "ord_pend") {
		t.Errorf("should not expose pending_payment: %s", rr2.Body.String())
	}
}
