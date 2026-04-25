package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminPendingLabels_ListsOnlyStuck(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()

	putPaidOrderForLabel(t, store, "ord_stuck_a")
	putPaidOrderForLabel(t, store, "ord_stuck_b")
	putPaidOrderForLabel(t, store, "ord_shipped")
	_ = store.setTracking(context.Background(), "ord_shipped", "BR0", "u", "l", "sf")
	// A pending-payment order — shouldn't appear either.
	putTestOrder(t, store, "ord_pending", "x@y.z", "card", 1000, 0)
	// Record a failure on one stuck order to see the metadata surface.
	_ = store.recordTrackingFailure(context.Background(), "ord_stuck_a", "checkout: wallet empty")

	h := adminAuth("adm", handleAdminPendingLabels(store))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders/pending-labels", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Count  int                 `json:"count"`
		Orders []adminPendingLabel `json:"orders"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Count != 2 {
		t.Fatalf("count = %d, want 2", got.Count)
	}
	ids := map[string]adminPendingLabel{}
	for _, o := range got.Orders {
		ids[o.OrderID] = o
	}
	if _, ok := ids["ord_stuck_a"]; !ok {
		t.Fatal("ord_stuck_a missing")
	}
	if _, ok := ids["ord_stuck_b"]; !ok {
		t.Fatal("ord_stuck_b missing")
	}
	if _, ok := ids["ord_shipped"]; ok {
		t.Fatal("shipped order should not appear")
	}
	if ids["ord_stuck_a"].TrackingAttempts != 1 {
		t.Fatalf("attempts = %d", ids["ord_stuck_a"].TrackingAttempts)
	}
	if ids["ord_stuck_a"].TrackingLastError == "" {
		t.Fatal("last error should be surfaced")
	}
}

func TestAdminPendingLabels_AuthRequired(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	h := adminAuth("adm", handleAdminPendingLabels(store))
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/admin/orders/pending-labels", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestAdminPendingLabels_MethodNotAllowed(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	h := adminAuth("adm", handleAdminPendingLabels(store))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/pending-labels", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rr.Code)
	}
}

func TestAdminRetryLabel_Success(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_retry")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	h := adminAuth("adm", handleAdminOrderActions(store, ship, 5*time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/ord_retry/retry-label", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got struct {
		OrderID      string `json:"orderId"`
		Status       string `json:"status"`
		TrackingCode string `json:"trackingCode"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.OrderID != "ord_retry" || got.Status != "shipped" || got.TrackingCode == "" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestAdminRetryLabel_Failure(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_retry_fail")

	fs := newFakeSuperFrete(t)
	fs.failCheckout = true
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	h := adminAuth("adm", handleAdminOrderActions(store, ship, 5*time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/ord_retry_fail/retry-label", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", rr.Code)
	}
}

func TestAdminRetryLabel_UnknownAction(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{AccessToken: "tok", BaseURL: "http://unused"})
	h := adminAuth("adm", handleAdminOrderActions(store, ship, time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/ord_x/bogus", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestAdminRetryLabel_MethodNotAllowed(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{AccessToken: "tok", BaseURL: "http://unused"})
	h := adminAuth("adm", handleAdminOrderActions(store, ship, time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders/ord_x/retry-label", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rr.Code)
	}
}
