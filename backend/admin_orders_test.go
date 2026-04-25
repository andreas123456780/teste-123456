package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), 5*time.Second))
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

func TestAdminRetryLabel_WithNameOverride(t *testing.T) {
	// Verifies that a JSON body {"name":"..."} passed to the retry
	// endpoint propagates all the way to the SuperFrete cart payload.
	// This is the exact recovery path used for orders where the
	// customer entered only a first name at checkout.
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_override")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), 5*time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/ord_override/retry-label",
		strings.NewReader(`{"name":"João Silva Santos"}`))
	req.Header.Set("X-Admin-Token", "adm")
	req.Header.Set("Content-Type", "application/json")
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	to, _ := fs.cartCapturedFields[0]["to"].(map[string]any)
	if got, _ := to["name"].(string); got != "João Silva Santos" {
		t.Fatalf("name override lost: to.name = %q", got)
	}
}

func TestAdminRetryLabel_InvalidJSON(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_badjson")
	ship := newShippingClient(shippingConfig{AccessToken: "tok", BaseURL: "http://unused"})
	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/ord_badjson/retry-label",
		strings.NewReader(`{"name": bad}`))
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for malformed body, got %d", rr.Code)
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

	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), 5*time.Second))
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
	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/ord_x/bogus", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestAdminOrdersList_ReturnsRecipientDetails(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_list_a")
	putPaidOrderForLabel(t, store, "ord_list_b")

	h := adminAuth("adm", handleAdminOrdersList(store))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders?limit=10", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Count  int                `json:"count"`
		Orders []adminOrderDetail `json:"orders"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Count < 2 {
		t.Fatalf("want >=2 orders, got %d", got.Count)
	}
	// Items must come back populated for the dashboard to list the
	// purchased products — that's the core contract of this endpoint.
	found := false
	for _, o := range got.Orders {
		if o.OrderID == "ord_list_a" {
			found = true
			if len(o.Items) == 0 {
				t.Errorf("expected items on ord_list_a, got 0")
			}
			if o.Name == "" || o.Email == "" {
				t.Errorf("expected name+email on ord_list_a, got %+v", o)
			}
		}
	}
	if !found {
		t.Fatal("ord_list_a not in listing")
	}
}

func TestAdminOrdersList_StatusFilter(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_paid_x")
	putTestOrder(t, store, "ord_pending_y", "x@y.z", "card", 1000, 0)

	h := adminAuth("adm", handleAdminOrdersList(store))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders?status=paid", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var got struct {
		Count  int                `json:"count"`
		Orders []adminOrderDetail `json:"orders"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	for _, o := range got.Orders {
		if o.Status != "paid" {
			t.Fatalf("status filter leaked %s (order %s)", o.Status, o.OrderID)
		}
	}
}

func TestAdminOrderDetail_Returns404ForMissing(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{AccessToken: "tok", BaseURL: "http://unused"})
	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders/ord_nope", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestAdminOrderDetail_IncludesItems(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_detail_a")
	ship := newShippingClient(shippingConfig{AccessToken: "tok", BaseURL: "http://unused"})
	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders/ord_detail_a", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got adminOrderDetail
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.OrderID != "ord_detail_a" {
		t.Fatalf("orderId = %q", got.OrderID)
	}
	if len(got.Items) == 0 {
		t.Error("expected items in detail response")
	}
}

func TestAdminOrderDelete_RemovesRow(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_del")
	ship := newShippingClient(shippingConfig{AccessToken: "tok", BaseURL: "http://unused"})
	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), time.Second))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/orders/ord_del", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
	_, ok, err := store.get(context.Background(), "ord_del")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if ok {
		t.Fatal("order still present after DELETE")
	}
}

func TestAdminOrderDelete_Returns404ForMissing(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{AccessToken: "tok", BaseURL: "http://unused"})
	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/orders/ord_nope", nil)
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
	h := adminAuth("adm", handleAdminOrderActions(store, ship, defaultFakeViaCep(t), time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders/ord_x/retry-label", nil)
	req.Header.Set("X-Admin-Token", "adm")
	h(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rr.Code)
	}
}
