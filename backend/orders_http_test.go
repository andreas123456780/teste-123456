package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleOrderLookup_HappyPath(t *testing.T) {
	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	o := putTestOrder(t, orders, "ord_abc", "andreas@example.com", "card", 11189, 2199)
	key := []byte("order-key")
	token := makeOrderToken(key, o.ID, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/orders/"+token, nil)
	rr := httptest.NewRecorder()
	handleOrderLookup(orders, key)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var out publicOrder
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.OrderID != o.ID {
		t.Fatalf("orderID=%q", out.OrderID)
	}
	if out.CustomerMasked == "" || out.CustomerMasked == "andreas@example.com" {
		t.Fatalf("email not masked: %q", out.CustomerMasked)
	}
	if out.AmountCents != 11189 {
		t.Fatalf("amount=%d", out.AmountCents)
	}
}

func TestHandleOrderLookup_InvalidToken(t *testing.T) {
	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	key := []byte("order-key")
	req := httptest.NewRequest(http.MethodGet, "/api/orders/not-a-token", nil)
	rr := httptest.NewRecorder()
	handleOrderLookup(orders, key)(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestHandleOrderLookup_NoKeyConfigured(t *testing.T) {
	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodGet, "/api/orders/any", nil)
	rr := httptest.NewRecorder()
	handleOrderLookup(orders, nil)(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestHandleOrderLookup_WrongMethod(t *testing.T) {
	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodPost, "/api/orders/foo", nil)
	rr := httptest.NewRecorder()
	handleOrderLookup(orders, []byte("k"))(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rr.Code)
	}
}
