package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseBRLCents(t *testing.T) {
	cases := []struct {
		in    string // as raw JSON, so `"12.34"` or `12.34`
		want  int
		okVal bool
	}{
		{`"12.34"`, 1234, true},
		{`"12,34"`, 1234, true},
		{`12.34`, 1234, true},
		{`"0.1"`, 10, true},
		{`"1"`, 100, true},
		{`"0"`, 0, true},
		{`0`, 0, true},
		{`null`, 0, false},
		{`""`, 0, false},
		{``, 0, false},
		{`"abc"`, 0, false},
		{`"-5.50"`, -550, true},
		{`"12.345"`, 1234, true}, // truncates to 2
	}
	for _, c := range cases {
		got, ok := parseBRLCents(json.RawMessage(c.in))
		if ok != c.okVal {
			t.Fatalf("parseBRLCents(%s) ok = %v, want %v", c.in, ok, c.okVal)
		}
		if ok && got != c.want {
			t.Fatalf("parseBRLCents(%s) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestValidBrazilianZip(t *testing.T) {
	cases := map[string]bool{
		"01310-100": true,
		"01310100":  true,
		"abc":       false,
		"1":         false,
		"123456789": false,
	}
	for in, want := range cases {
		if got := validBrazilianZip(in); got != want {
			t.Fatalf("validBrazilianZip(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBuildPackages_MergesQuantities(t *testing.T) {
	items := []ShippingCartRef{
		{ProductID: "p-tee-bw-black", Quantity: 1},
		{ProductID: "p-boxy-black", Quantity: 2},
		{ProductID: "p-tee-bw-black", Quantity: 3}, // merges with first
	}
	out, err := buildPackages(items)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 packages, got %d", len(out))
	}
	if out[0].ID != "p-tee-bw-black" || out[0].Quantity != 4 {
		t.Fatalf("first pkg = %+v", out[0])
	}
	if out[1].ID != "p-boxy-black" || out[1].Quantity != 2 {
		t.Fatalf("second pkg = %+v", out[1])
	}
	// Dimensions must come from the preset, not fallback.
	if out[0].Weight != 0.22 {
		t.Fatalf("expected weight from preset, got %v", out[0].Weight)
	}
}

func TestBuildPackages_RejectsBadInput(t *testing.T) {
	if _, err := buildPackages(nil); err == nil {
		t.Fatal("expected error on empty")
	}
	if _, err := buildPackages([]ShippingCartRef{{ProductID: "", Quantity: 1}}); err == nil {
		t.Fatal("expected error on empty product id")
	}
	if _, err := buildPackages([]ShippingCartRef{{ProductID: "x", Quantity: 0}}); err == nil {
		t.Fatal("expected error on zero quantity")
	}
	if _, err := buildPackages([]ShippingCartRef{{ProductID: "x", Quantity: 9999}}); err == nil {
		t.Fatal("expected error on huge quantity")
	}
}

func TestNormalizeQuotes(t *testing.T) {
	raw := []meQuote{
		{ID: 1, Name: "PAC", Price: json.RawMessage(`"19.90"`), DeliveryRange: struct {
			Min int `json:"min"`
			Max int `json:"max"`
		}{Min: 5, Max: 8}, Company: struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Picture string `json:"picture"`
		}{ID: 1, Name: "Correios"}},
		{ID: 2, Name: "Jadlog .Package", Price: json.RawMessage(`22.5`), DeliveryTime: 4, Company: struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Picture string `json:"picture"`
		}{ID: 2, Name: "Jadlog"}},
		{ID: 3, Name: "Loggi", Error: "CEP não atendido"},
	}
	got := normalizeQuotes(raw)
	if len(got) != 3 {
		t.Fatalf("want 3 options, got %d", len(got))
	}
	if got[0].PriceCents != 1990 {
		t.Fatalf("PAC cents = %d", got[0].PriceCents)
	}
	if got[1].PriceCents != 2250 {
		t.Fatalf("Jadlog cents = %d", got[1].PriceCents)
	}
	if got[1].DeliveryMin != 4 || got[1].DeliveryMax != 4 {
		t.Fatalf("Jadlog delivery = %d/%d", got[1].DeliveryMin, got[1].DeliveryMax)
	}
	if got[2].Error == "" {
		t.Fatal("expected error preserved")
	}
	if got[2].PriceCents != 0 {
		t.Fatalf("erroring carrier should have 0 cents, got %d", got[2].PriceCents)
	}
}

func TestShippingQuote_EndToEnd(t *testing.T) {
	// Fake Melhor Envio: check inbound auth + payload, return a canned quote.
	var captured meCalculateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/me/shipment/calculate" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "NAST") {
			t.Errorf("User-Agent = %q", ua)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`[
			{"id":1,"name":"PAC","price":"21.50","delivery_range":{"min":5,"max":9},"company":{"id":1,"name":"Correios"}},
			{"id":2,"name":"SEDEX","price":"35.00","delivery_range":{"min":2,"max":4},"company":{"id":1,"name":"Correios"}}
		]`))
	}))
	defer srv.Close()

	c := newShippingClient(shippingConfig{
		BaseURL:     srv.URL,
		AccessToken: "test-token",
		UserAgent:   "NAST Test (t@t.co)",
		OriginZip:   "01310100",
	})
	cache := newQuoteCache(0) // disable cache

	handler := handleShippingQuote(c, cache)

	body, _ := json.Marshal(ShippingQuoteRequest{
		ZipCode: "04567-000",
		Items: []ShippingCartRef{
			{ProductID: "p-tee-bw-black", Quantity: 1},
			{ProductID: "p-boxy-black", Quantity: 2},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/shipping/quote", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Options []ShippingQuoteOption `json:"options"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Options) != 2 {
		t.Fatalf("want 2 options, got %d", len(out.Options))
	}
	if out.Options[0].PriceCents != 2150 {
		t.Fatalf("pac cents = %d", out.Options[0].PriceCents)
	}
	if out.Options[1].ServiceName != "SEDEX" || out.Options[1].PriceCents != 3500 {
		t.Fatalf("sedex = %+v", out.Options[1])
	}
	// Verify upstream request was well-formed.
	if captured.From.PostalCode != "01310100" {
		t.Fatalf("from zip = %q", captured.From.PostalCode)
	}
	if captured.To.PostalCode != "04567000" {
		t.Fatalf("to zip = %q (should be digits-only)", captured.To.PostalCode)
	}
	if len(captured.Products) != 2 {
		t.Fatalf("want 2 consolidated packages, got %d", len(captured.Products))
	}
}

func TestShippingQuote_InvalidZip(t *testing.T) {
	c := newShippingClient(shippingConfig{BaseURL: "http://unused", AccessToken: "t", OriginZip: "01310100"})
	handler := handleShippingQuote(c, newQuoteCache(0))

	body, _ := json.Marshal(ShippingQuoteRequest{
		ZipCode: "abc",
		Items:   []ShippingCartRef{{ProductID: "p-tee-bw-black", Quantity: 1}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/shipping/quote", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestShippingQuote_Unconfigured(t *testing.T) {
	c := newShippingClient(shippingConfig{BaseURL: "http://unused", AccessToken: ""})
	handler := handleShippingQuote(c, newQuoteCache(0))
	req := httptest.NewRequest(http.MethodPost, "/api/shipping/quote", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestShippingQuote_CachesEqualRequests(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`[{"id":1,"name":"PAC","price":"10.00","delivery_range":{"min":5,"max":9},"company":{"id":1,"name":"Correios"}}]`))
	}))
	defer srv.Close()
	c := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "t", UserAgent: "NAST", OriginZip: "01310100"})
	cache := newQuoteCache(60 * time.Second)

	body, _ := json.Marshal(ShippingQuoteRequest{
		ZipCode: "04567-000",
		Items:   []ShippingCartRef{{ProductID: "p-tee-bw-black", Quantity: 1}},
	})
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/shipping/quote", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handleShippingQuote(c, cache)(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("iter %d: status = %d", i, rr.Code)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 1 upstream call (cached), got %d", calls)
	}
}

func TestRequireAdmin_ConstantTimeCompare(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if requireAdmin("", rr, req) {
		t.Fatal("empty admin token should block")
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/x", nil)
	if requireAdmin("secret", rr, req) {
		t.Fatal("missing header should block")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("X-Admin-Token", "secret")
	if !requireAdmin("secret", rr, req) {
		t.Fatal("matching header should allow")
	}
}
