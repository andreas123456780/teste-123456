package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestInternalJobsAuth_Disabled503(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{AccessToken: "x", BaseURL: "http://unused"})
	h := handleProcessLabelsJob(internalJobsConfig{}, store, ship, defaultFakeViaCep(t))

	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodPost, "/api/internal/jobs/process-labels", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when INTERNAL_JOB_TOKEN unset, got %d", rr.Code)
	}
}

func TestInternalJobsAuth_MissingBearer(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{AccessToken: "x", BaseURL: "http://unused"})
	h := handleProcessLabelsJob(internalJobsConfig{token: "secret"}, store, ship, defaultFakeViaCep(t))

	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodPost, "/api/internal/jobs/process-labels", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no Authorization, got %d", rr.Code)
	}
}

func TestInternalJobsAuth_WrongToken(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{AccessToken: "x", BaseURL: "http://unused"})
	h := handleProcessLabelsJob(internalJobsConfig{token: "secret"}, store, ship, defaultFakeViaCep(t))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/jobs/process-labels", nil)
	req.Header.Set("Authorization", "Bearer nope")
	h(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", rr.Code)
	}
}

func TestInternalJobsShippingUnconfigured503(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	// shipping client without AccessToken
	ship := newShippingClient(shippingConfig{})
	h := handleProcessLabelsJob(internalJobsConfig{token: "secret"}, store, ship, defaultFakeViaCep(t))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/jobs/process-labels", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when SuperFrete unconfigured, got %d", rr.Code)
	}
}

func TestProcessLabelsJob_HappyPath(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()

	putPaidOrderForLabel(t, store, "ord_cron_a")
	putPaidOrderForLabel(t, store, "ord_cron_b")
	// One already shipped — should be ignored by the listing.
	putPaidOrderForLabel(t, store, "ord_cron_done")
	_ = store.setTracking(context.Background(), "ord_cron_done", "BR999", "u", "l", "sf_done")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{
		BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000", From: testSenderAddr(), Autopay: true,
	})
	h := handleProcessLabelsJob(internalJobsConfig{token: "secret"}, store, ship, defaultFakeViaCep(t))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/jobs/process-labels", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got processLabelsResult
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if got.Scanned != 2 {
		t.Fatalf("scanned = %d, want 2", got.Scanned)
	}
	if got.Succeeded != 2 {
		t.Fatalf("succeeded = %d, want 2; errors=%v", got.Succeeded, got.Errors)
	}
	if atomic.LoadInt32(&fs.cartCalls) != 2 {
		t.Fatalf("cart calls = %d, want 2", fs.cartCalls)
	}

	// Confirm both orders are now shipped with tracking.
	for _, id := range []string{"ord_cron_a", "ord_cron_b"} {
		o, _, _ := store.get(context.Background(), id)
		if o.Status != "shipped" || o.TrackingCode == "" {
			t.Fatalf("%s: status=%q tracking=%q", id, o.Status, o.TrackingCode)
		}
	}
}

func TestProcessLabelsJob_SkipsRecentFailures(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()

	putPaidOrderForLabel(t, store, "ord_backoff")
	// Freshly failed — attempt count > 0 puts it in backoff.
	_ = store.recordTrackingFailure(context.Background(), "ord_backoff", "boom")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{
		BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000", From: testSenderAddr(),
	})
	h := handleProcessLabelsJob(internalJobsConfig{token: "secret"}, store, ship, defaultFakeViaCep(t))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/jobs/process-labels", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	// listPendingLabels filter should hide rows attempted more recently
	// than labelBackoff(1), so the cron reports 0 scanned.
	var got processLabelsResult
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Scanned != 0 {
		t.Fatalf("scanned = %d, want 0 (row in backoff)", got.Scanned)
	}
	if atomic.LoadInt32(&fs.cartCalls) != 0 {
		t.Fatalf("SuperFrete should not be called during backoff, got %d calls", fs.cartCalls)
	}
}

func TestProcessLabelsJob_MethodNotAllowed(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{AccessToken: "x", BaseURL: "http://unused"})
	h := handleProcessLabelsJob(internalJobsConfig{token: "secret"}, store, ship, defaultFakeViaCep(t))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/internal/jobs/process-labels", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rr.Code)
	}
}

func TestLabelTimeoutFromEnv_Default(t *testing.T) {
	t.Setenv("LABEL_PROCESSING_TIMEOUT", "")
	if got := labelTimeoutFromEnv(); got != 25*time.Second {
		t.Fatalf("default = %v, want 25s", got)
	}
}

func TestLabelTimeoutFromEnv_Override(t *testing.T) {
	t.Setenv("LABEL_PROCESSING_TIMEOUT", "15s")
	if got := labelTimeoutFromEnv(); got != 15*time.Second {
		t.Fatalf("override = %v, want 15s", got)
	}
}

func TestLabelTimeoutFromEnv_InvalidFallsBack(t *testing.T) {
	t.Setenv("LABEL_PROCESSING_TIMEOUT", "not a duration")
	if got := labelTimeoutFromEnv(); got != 25*time.Second {
		t.Fatalf("fallback on invalid = %v, want 25s", got)
	}
}
