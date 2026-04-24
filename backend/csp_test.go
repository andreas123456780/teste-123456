package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildCSP_APIOnly(t *testing.T) {
	got := buildCSP(false, "")
	if !strings.Contains(got, "default-src 'none'") {
		t.Fatalf("API-only CSP should start from default-src 'none'; got %q", got)
	}
	if strings.Contains(got, "stripe") {
		t.Fatalf("API-only CSP must not allow Stripe; got %q", got)
	}
}

func TestBuildCSP_SPAIncludesStripe(t *testing.T) {
	got := buildCSP(true, "")
	for _, want := range []string{"js.stripe.com", "hooks.stripe.com", "frame-ancestors 'none'", "form-action 'self'"} {
		if !strings.Contains(got, want) {
			t.Fatalf("SPA CSP missing %q: %q", want, got)
		}
	}
}

func TestBuildCSP_SPAWithPlausible(t *testing.T) {
	got := buildCSP(true, "https://plausible.io/js/script.js")
	if !strings.Contains(got, "https://plausible.io") {
		t.Fatalf("plausible script src missing: %q", got)
	}
}

func TestPlausibleOrigin(t *testing.T) {
	cases := map[string]string{
		"https://plausible.io/js/script.js":    "https://plausible.io",
		"https://pa.example.com/script.js?v=1": "https://pa.example.com",
		"":                                     "",
		"not a url":                            "",
	}
	for in, want := range cases {
		if got := plausibleOrigin(in); got != want {
			t.Fatalf("plausibleOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithRecovery_RecoversAndReturns500(t *testing.T) {
	var panicker http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	h := withRecovery(panicker)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on panic, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "internal error") {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}
