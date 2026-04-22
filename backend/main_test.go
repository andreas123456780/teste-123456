package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP_UntrustedPeer_IgnoresXFF(t *testing.T) {
	trusted := parseTrustedProxies("10.0.0.0/8")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:45012"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := clientIP(req, trusted); got != "203.0.113.9" {
		t.Fatalf("untrusted peer should ignore XFF; got %q", got)
	}
}

func TestClientIP_TrustedPeer_WalksRightToLeft(t *testing.T) {
	trusted := parseTrustedProxies("10.0.0.0/8,172.16.0.0/12")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:45012"
	// Spoof attempt: attacker prepended a fake IP; the real chain appended
	// by the proxy ends with the actual client then the internal hops.
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 203.0.113.9, 172.16.0.5")
	if got := clientIP(req, trusted); got != "203.0.113.9" {
		t.Fatalf("trusted peer should skip trusted hops and return the real client; got %q", got)
	}
}

func TestClientIP_EmptyTrusted_UsesPeer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:45012"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := clientIP(req, nil); got != "203.0.113.9" {
		t.Fatalf("default (no trusted proxies) must use peer; got %q", got)
	}
}

func TestClientIP_TrustedPeer_NoXFF(t *testing.T) {
	trusted := parseTrustedProxies("10.0.0.0/8")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:45012"
	if got := clientIP(req, trusted); got != "10.0.0.2" {
		t.Fatalf("trusted peer with no XFF should fall back to peer; got %q", got)
	}
}

func TestClientIP_AllTrustedChain_FallsBackToPeer(t *testing.T) {
	trusted := parseTrustedProxies("10.0.0.0/8")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:45012"
	req.Header.Set("X-Forwarded-For", "10.0.0.3, 10.0.0.4")
	if got := clientIP(req, trusted); got != "10.0.0.2" {
		t.Fatalf("entire chain trusted should bill peer; got %q", got)
	}
}

func TestHealth(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handleHealth(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestProductsList(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	handleProducts(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var out []Product
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected products, got 0")
	}
}

func TestProductByID_NotFound(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/products/does-not-exist", nil)
	handleProductByID(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestCheckout_ValidCard(t *testing.T) {
	body := CheckoutRequest{
		Items:         []CartItem{{ProductID: "p-tee-bw-black", Quantity: 2, Size: "M", Color: "preto"}},
		Name:          "Andreas Teste",
		Email:         "andreas@example.com",
		Address:       "Rua das Flores, 123",
		ZipCode:       "01000-000",
		PaymentMethod: "card",
	}
	b, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(b))
	handleCheckout(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var out CheckoutResponse
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.TotalCents != 8990*2 {
		t.Fatalf("unexpected total %d (want %d)", out.TotalCents, 8990*2)
	}
}

func TestCheckout_ValidPix(t *testing.T) {
	body := CheckoutRequest{
		Items:         []CartItem{{ProductID: "p-tee-bw-black", Quantity: 1, Size: "M", Color: "preto"}},
		Name:          "Andreas Teste",
		Email:         "andreas@example.com",
		Address:       "Rua das Flores, 123",
		ZipCode:       "01000-000",
		PaymentMethod: "pix",
	}
	b, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(b))
	handleCheckout(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var out CheckoutResponse
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.TotalCents != 8541 {
		t.Fatalf("unexpected total %d (want 8541 pix)", out.TotalCents)
	}
}

func TestCheckout_InvalidPaymentMethod(t *testing.T) {
	body := CheckoutRequest{
		Items:         []CartItem{{ProductID: "p-tee-bw-black", Quantity: 1, Size: "M", Color: "preto"}},
		Name:          "Andreas Teste",
		Email:         "andreas@example.com",
		Address:       "Rua 1",
		ZipCode:       "01000",
		PaymentMethod: "crypto",
	}
	b, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(b))
	handleCheckout(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCheckout_InvalidEmail(t *testing.T) {
	body := CheckoutRequest{
		Items:   []CartItem{{ProductID: "p-tee-bw-black", Quantity: 1}},
		Name:    "x",
		Email:   "not-an-email",
		Address: "Rua 1",
		ZipCode: "01000",
	}
	b, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(b))
	handleCheckout(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(2, 0.0001)
	if !rl.allow("1.2.3.4") {
		t.Fatal("first call should pass")
	}
	if !rl.allow("1.2.3.4") {
		t.Fatal("second call should pass")
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("third call should be limited")
	}
}
