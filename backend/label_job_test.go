package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSuperFrete is a configurable httptest server that mimics the
// SuperFrete endpoints runLabelJob walks through. Fields let each test
// override individual hops to assert error handling.
type fakeSuperFrete struct {
	t                  *testing.T
	cartID             string
	cartCalls          int32
	checkoutCalls      int32
	generateCalls      int32
	printCalls         int32
	orderInfoCalls     int32
	failCart           bool
	failCheckout       bool
	failPrint          bool
	infoBody           string
	printBody          string
	cartCapturedFields []map[string]any
}

func newFakeSuperFrete(t *testing.T) *fakeSuperFrete {
	return &fakeSuperFrete{
		t:         t,
		cartID:    "sf_cart_123",
		infoBody:  `{"tracking":"BR123456789BR","tracking_url":"https://rastreio.com/BR123456789BR"}`,
		printBody: `{"url":"https://superfrete.example/labels/abc.pdf"}`,
	}
}

func (f *fakeSuperFrete) start() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v0/cart", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.cartCalls, 1)
		if f.failCart {
			http.Error(w, `{"error":"cart bad"}`, http.StatusBadRequest)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var got map[string]any
		_ = json.Unmarshal(raw, &got)
		f.cartCapturedFields = append(f.cartCapturedFields, got)
		_, _ = w.Write([]byte(`{"id":"` + f.cartID + `"}`))
	})
	mux.HandleFunc("/api/v0/checkout", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.checkoutCalls, 1)
		if f.failCheckout {
			http.Error(w, `{"error":"checkout failed"}`, http.StatusPaymentRequired)
			return
		}
		_, _ = w.Write([]byte(`{"purchase":"ok"}`))
	})
	mux.HandleFunc("/api/v0/generate", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.generateCalls, 1)
		_, _ = w.Write([]byte(`{"generate":"ok"}`))
	})
	mux.HandleFunc("/api/v0/print", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.printCalls, 1)
		if f.failPrint {
			http.Error(w, `{"error":"print failed"}`, http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(f.printBody))
	})
	mux.HandleFunc("/api/v0/order/info/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.orderInfoCalls, 1)
		_, _ = w.Write([]byte(f.infoBody))
	})
	return httptest.NewServer(mux)
}

// putPaidOrderForLabel inserts a paid order with a known shipping
// service + product so runLabelJob has enough context to build a cart.
// It uses the first product from the in-code catalog to inherit a real
// packaging preset.
func putPaidOrderForLabel(t *testing.T, store *orderStore, id string) {
	t.Helper()
	if len(catalog) == 0 {
		t.Fatal("catalog is empty; tests need at least one product preset")
	}
	prod := catalog[0]
	o := &pendingOrder{
		ID:              id,
		Name:            "Bianca Marques",
		Email:           "bianca@example.com",
		Address:         "Rua das Bandeiras, 123",
		Zip:             "08503-000",
		PaymentMethod:   "card",
		Status:          "paid",
		TotalCents:      prod.PriceCents,
		ShippingCents:   2000,
		AmountCents:     prod.PriceCents + 2000,
		ShippingSvcID:   1,
		ShippingSvcName: "PAC",
		PaymentIntentID: "pi_" + id,
		CreatedAt:       time.Now().UTC(),
		Items: []orderItem{
			{
				ProductID:      prod.ID,
				ProductName:    prod.Name,
				Size:           "M",
				Quantity:       1,
				UnitPriceCents: prod.PriceCents,
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := store.create(ctx, o); err != nil {
		t.Fatalf("create order: %v", err)
	}
}

func TestRunLabelJob_HappyPath(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_happy")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{
		BaseURL:     srv.URL,
		AccessToken: "tok",
		UserAgent:   "NAST",
		OriginZip:   "08503000",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runLabelJob(ctx, store, ship, defaultFakeViaCep(t), "ord_happy", labelOverrides{}); err != nil {
		t.Fatalf("runLabelJob: %v", err)
	}

	o, ok, err := store.get(ctx, "ord_happy")
	if err != nil || !ok {
		t.Fatalf("get: err=%v ok=%v", err, ok)
	}
	if o.Status != "shipped" {
		t.Fatalf("status = %q, want shipped", o.Status)
	}
	if o.TrackingCode != "BR123456789BR" {
		t.Fatalf("tracking_code = %q", o.TrackingCode)
	}
	if !strings.Contains(o.LabelURL, "abc.pdf") {
		t.Fatalf("label_url = %q", o.LabelURL)
	}
	if o.SuperfreteID != fs.cartID {
		t.Fatalf("superfrete_order_id = %q", o.SuperfreteID)
	}
	if o.TrackingLastError != "" {
		t.Fatalf("expected no last error after success, got %q", o.TrackingLastError)
	}
}

func TestRunLabelJob_IdempotentSkip(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_done")

	ctx := context.Background()
	if err := store.setTracking(ctx, "ord_done", "ALREADYSHIPPED", "https://x", "https://y", "sf_existing"); err != nil {
		t.Fatalf("setTracking: %v", err)
	}

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	if err := runLabelJob(ctx, store, ship, defaultFakeViaCep(t), "ord_done", labelOverrides{}); err != nil {
		t.Fatalf("runLabelJob on already-tracked order: %v", err)
	}
	if atomic.LoadInt32(&fs.cartCalls) != 0 {
		t.Fatalf("expected SuperFrete to not be called when tracking already set, got %d cart calls", fs.cartCalls)
	}
}

func TestRunLabelJob_CheckoutFailureRecordsAttempt(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_fail")

	fs := newFakeSuperFrete(t)
	fs.failCheckout = true
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	ctx := context.Background()
	err := runLabelJob(ctx, store, ship, defaultFakeViaCep(t), "ord_fail", labelOverrides{})
	if err == nil {
		t.Fatal("expected error from runLabelJob, got nil")
	}
	if !strings.Contains(err.Error(), "checkout") {
		t.Fatalf("error should mention failing stage, got %v", err)
	}

	o, ok, _ := store.get(ctx, "ord_fail")
	if !ok {
		t.Fatal("order missing")
	}
	if o.Status != "paid" {
		t.Fatalf("status changed to %q on failure (should stay paid)", o.Status)
	}
	if o.TrackingAttempts != 1 {
		t.Fatalf("tracking_attempts = %d, want 1", o.TrackingAttempts)
	}
	if !strings.Contains(o.TrackingLastError, "checkout") {
		t.Fatalf("tracking_last_error = %q", o.TrackingLastError)
	}
	if o.TrackingAttemptedAt.IsZero() {
		t.Fatal("tracking_attempted_at not set")
	}
}

func TestRunLabelJob_OrderNotFound(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{BaseURL: "http://unused", AccessToken: "tok", OriginZip: "08503000"})
	err := runLabelJob(context.Background(), store, ship, defaultFakeViaCep(t), "ord_missing", labelOverrides{})
	if err == nil {
		t.Fatal("expected error for unknown order")
	}
}

func TestRunLabelJob_ShippingNotConfigured(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{}) // no AccessToken
	if err := runLabelJob(context.Background(), store, ship, defaultFakeViaCep(t), "anything", labelOverrides{}); err == nil {
		t.Fatal("expected error when SuperFrete unconfigured")
	}
}

func TestRunLabelJob_SkipsUnpaidOrder(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	// pending_payment status — should be skipped silently.
	putTestOrder(t, store, "ord_pending", "x@y.z", "card", 5000, 1000)
	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})
	if err := runLabelJob(context.Background(), store, ship, defaultFakeViaCep(t), "ord_pending", labelOverrides{}); err != nil {
		t.Fatalf("runLabelJob: %v", err)
	}
	if atomic.LoadInt32(&fs.cartCalls) != 0 {
		t.Fatal("SuperFrete should not be hit for non-paid orders")
	}
}

func TestSyncLabelDispatcher_RunsSynchronously(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_sync")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	d := newSyncLabelDispatcher(store, ship, 5*time.Second)
	d.viacep = defaultFakeViaCep(t)
	d.Enqueue("ord_sync") // returns only after the SuperFrete pipeline finishes

	if atomic.LoadInt32(&fs.cartCalls) == 0 {
		t.Fatal("expected SuperFrete to be hit synchronously inside Enqueue")
	}
	o, _, _ := store.get(context.Background(), "ord_sync")
	if o.TrackingCode == "" {
		t.Fatalf("tracking_code still empty after Enqueue; status=%q lastErr=%q", o.Status, o.TrackingLastError)
	}
}

func TestSyncLabelDispatcher_NoTokenSkips(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	ship := newShippingClient(shippingConfig{}) // no token
	d := newSyncLabelDispatcher(store, ship, time.Second)
	// Should not panic and should not write anything to the order.
	d.Enqueue("anything")
}

func TestRecordTrackingFailureIncrements(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_count")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := store.recordTrackingFailure(ctx, "ord_count", "boom"); err != nil {
			t.Fatalf("recordTrackingFailure: %v", err)
		}
	}
	o, _, _ := store.get(ctx, "ord_count")
	if o.TrackingAttempts != 3 {
		t.Fatalf("tracking_attempts = %d", o.TrackingAttempts)
	}
}

func TestListPendingLabels_FiltersByStatusAndTracking(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_a")
	putPaidOrderForLabel(t, store, "ord_b")
	putTestOrder(t, store, "ord_pending", "x@y.z", "card", 1000, 0) // pending_payment, ignored

	ctx := context.Background()
	// One of them already has tracking.
	if err := store.setTracking(ctx, "ord_b", "BR000", "u", "l", "sf"); err != nil {
		t.Fatalf("setTracking: %v", err)
	}

	got, err := store.listPendingLabels(ctx, time.Now().UTC(), 10, 50)
	if err != nil {
		t.Fatalf("listPendingLabels: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ord_a" {
		ids := []string{}
		for _, o := range got {
			ids = append(ids, o.ID)
		}
		t.Fatalf("expected only ord_a, got %v", ids)
	}
}

func TestListPendingLabels_RespectsCutoff(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_recent")
	ctx := context.Background()
	// Mark a fresh attempt; cutoff strictly before now should hide it.
	if err := store.recordTrackingFailure(ctx, "ord_recent", "x"); err != nil {
		t.Fatalf("recordTrackingFailure: %v", err)
	}
	got, err := store.listPendingLabels(ctx, time.Now().UTC().Add(-10*time.Minute), 10, 50)
	if err != nil {
		t.Fatalf("listPendingLabels: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 with stale cutoff, got %d", len(got))
	}
	// With now() as cutoff, the row appears.
	got, _ = store.listPendingLabels(ctx, time.Now().UTC(), 10, 50)
	if len(got) != 1 {
		t.Fatalf("expected 1 with now cutoff, got %d", len(got))
	}
}

func TestListPendingLabels_RespectsMaxAttempts(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_capped")
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_ = store.recordTrackingFailure(ctx, "ord_capped", "x")
	}
	got, _ := store.listPendingLabels(ctx, time.Now().UTC(), 4, 50)
	if len(got) != 0 {
		t.Fatalf("expected 0 (attempts >= max), got %d", len(got))
	}
	got, _ = store.listPendingLabels(ctx, time.Now().UTC(), 5, 50)
	if len(got) != 1 {
		t.Fatalf("expected 1 with higher cap, got %d", len(got))
	}
}

func TestLabelBackoff_Monotonic(t *testing.T) {
	prev := time.Duration(0)
	for i := 0; i <= 8; i++ {
		got := labelBackoff(i)
		if got < prev {
			t.Fatalf("backoff(%d) = %v < backoff prev = %v", i, got, prev)
		}
		prev = got
	}
	if labelBackoff(20) > time.Hour {
		t.Fatalf("backoff(20) = %v exceeds cap", labelBackoff(20))
	}
}

// TestRunLabelJob_SendsDistrictCityStateToSuperFrete guards against
// the exact regression that first caused this work: SuperFrete rejects
// cart payloads that don't include recipient district/city/state. The
// fields get resolved either from overrides or a ViaCEP lookup against
// the order's zip — this test wires up both and asserts the payload.
func TestRunLabelJob_SendsDistrictCityStateToSuperFrete(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_addr")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	viacep := newFakeViaCep(t, addressDetails{District: "Jardim América", City: "Suzano", State: "SP"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runLabelJob(ctx, store, ship, viacep, "ord_addr", labelOverrides{}); err != nil {
		t.Fatalf("runLabelJob: %v", err)
	}
	if len(fs.cartCapturedFields) != 1 {
		t.Fatalf("expected exactly one cart call, got %d", len(fs.cartCapturedFields))
	}
	to, _ := fs.cartCapturedFields[0]["to"].(map[string]any)
	if to == nil {
		t.Fatal("cart payload missing 'to' object")
	}
	if got, _ := to["district"].(string); got != "Jardim América" {
		t.Fatalf("to.district = %q, want Jardim América", got)
	}
	if got, _ := to["city"].(string); got != "Suzano" {
		t.Fatalf("to.city = %q, want Suzano", got)
	}
	if got, _ := to["state_abbr"].(string); got != "SP" {
		t.Fatalf("to.state_abbr = %q, want SP", got)
	}
}

// TestRunLabelJob_DocumentOverrideReachesSuperFrete covers recipient
// CPF/CNPJ, which SuperFrete now requires on most accounts. Digits-
// only normalisation is validated so the operator can paste
// "123.456.789-00" or "12345678900" and both work.
func TestRunLabelJob_DocumentOverrideReachesSuperFrete(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_doc")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runLabelJob(ctx, store, ship, defaultFakeViaCep(t), "ord_doc", labelOverrides{
		Document: "123.456.789-00",
	})
	if err != nil {
		t.Fatalf("runLabelJob: %v", err)
	}
	to, _ := fs.cartCapturedFields[0]["to"].(map[string]any)
	if got, _ := to["document"].(string); got != "12345678900" {
		t.Fatalf("to.document = %q, want 12345678900 (digits-only)", got)
	}
}

// TestRunLabelJob_NameOverrideReachesSuperFrete covers the manual-
// recovery path where an operator retries a stuck order with a
// corrected recipient name (SuperFrete rejects first-name-only).
func TestRunLabelJob_NameOverrideReachesSuperFrete(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_name")

	fs := newFakeSuperFrete(t)
	srv := fs.start()
	defer srv.Close()
	ship := newShippingClient(shippingConfig{BaseURL: srv.URL, AccessToken: "tok", OriginZip: "08503000"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runLabelJob(ctx, store, ship, defaultFakeViaCep(t), "ord_name", labelOverrides{
		Name: "João da Silva Santos",
	})
	if err != nil {
		t.Fatalf("runLabelJob: %v", err)
	}
	to, _ := fs.cartCapturedFields[0]["to"].(map[string]any)
	if got, _ := to["name"].(string); got != "João da Silva Santos" {
		t.Fatalf("to.name = %q, override ignored", got)
	}
}

// Reuse the existing pendingOrder.errors check to guard against a
// regression where wrapAndRecord swallows the original cause.
func TestWrapAndRecordPreservesCause(t *testing.T) {
	store, _, cleanup := newTestStore(t)
	defer cleanup()
	putPaidOrderForLabel(t, store, "ord_wrap")
	got := wrapAndRecord(context.Background(), store, "ord_wrap", "stage", errors.New("downstream"))
	if !strings.Contains(got.Error(), "downstream") {
		t.Fatalf("error chain lost cause: %v", got)
	}
	if !strings.Contains(got.Error(), "stage") {
		t.Fatalf("error chain lost stage: %v", got)
	}
}
