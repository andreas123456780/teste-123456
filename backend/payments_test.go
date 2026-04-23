package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVerifyStripeSignature_OK(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_123","type":"payment_intent.succeeded"}`)
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	header := "t=" + strconv.FormatInt(ts, 10) + ",v1=" + sig
	if err := verifyStripeSignature(secret, header, payload, time.Now()); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifyStripeSignature_BadSig(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_123"}`)
	ts := time.Now().Unix()
	header := "t=" + strconv.FormatInt(ts, 10) + ",v1=deadbeef"
	if err := verifyStripeSignature(secret, header, payload, time.Now()); err == nil {
		t.Fatal("expected signature mismatch error")
	}
}

func TestVerifyStripeSignature_StaleTimestamp(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{}`)
	ts := time.Now().Add(-1 * time.Hour).Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	header := "t=" + strconv.FormatInt(ts, 10) + ",v1=" + sig
	if err := verifyStripeSignature(secret, header, payload, time.Now()); err == nil {
		t.Fatal("expected stale-timestamp error")
	}
}

func TestVerifyStripeSignature_NoSecret(t *testing.T) {
	if err := verifyStripeSignature("", "t=1,v1=abc", []byte("{}"), time.Now()); err == nil {
		t.Fatal("expected error when secret missing")
	}
}

func TestHandlePaymentsIntent_CreatesPI(t *testing.T) {
	// Fake Stripe: accept POST /v1/payment_intents, return canned PI.
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payment_intents" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk_test" {
			t.Errorf("Authorization = %q", got)
		}
		captured, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"id":"pi_123","client_secret":"pi_123_secret_xyz","status":"requires_payment_method","amount":11189,"currency":"brl"}`))
	}))
	defer srv.Close()

	stripe := &stripeClient{
		cfg:  paymentsConfig{SecretKey: "sk_test"},
		http: srv.Client(),
	}
	// Override base for this test via a small shim: re-use a client whose
	// outbound URL points at our fake server.
	origBase := stripeAPIBaseOverride
	stripeAPIBaseOverride = srv.URL
	defer func() { stripeAPIBaseOverride = origBase }()

	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	putTestOrder(t, orders, "ord_abc", "andreas@example.com", "card", 11189, 2199)

	handler := handlePaymentsIntent(stripe, orders)
	body, _ := json.Marshal(paymentIntentRequest{OrderID: "ord_abc"})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/intent", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var out paymentIntentResponse
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ClientSecret != "pi_123_secret_xyz" {
		t.Fatalf("client secret = %q", out.ClientSecret)
	}
	if out.AmountCents != 11189 {
		t.Fatalf("amount = %d", out.AmountCents)
	}
	body2 := string(captured)
	if !strings.Contains(body2, "amount=11189") {
		t.Fatalf("expected amount in body, got %q", body2)
	}
	if !strings.Contains(body2, "currency=brl") {
		t.Fatalf("expected brl currency, got %q", body2)
	}
	if !strings.Contains(body2, "payment_method_types%5B%5D=card") {
		t.Fatalf("expected card method, got %q", body2)
	}
	if !strings.Contains(body2, "metadata%5BorderId%5D=ord_abc") {
		t.Fatalf("expected orderId metadata, got %q", body2)
	}
	// Order should now have the PI id attached.
	o, ok, err := orders.get(context.Background(), "ord_abc")
	if err != nil || !ok {
		t.Fatalf("get order: err=%v ok=%v", err, ok)
	}
	if o.PaymentIntentID != "pi_123" {
		t.Fatalf("expected PI id attached to order, got %q", o.PaymentIntentID)
	}
}

func TestHandlePaymentsIntent_UnknownOrder(t *testing.T) {
	stripe := &stripeClient{cfg: paymentsConfig{SecretKey: "sk_test"}, http: http.DefaultClient}
	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	body, _ := json.Marshal(paymentIntentRequest{OrderID: "ord_missing"})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/intent", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handlePaymentsIntent(stripe, orders)(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestHandlePaymentsIntent_Unconfigured(t *testing.T) {
	stripe := &stripeClient{cfg: paymentsConfig{}, http: http.DefaultClient}
	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodPost, "/api/payments/intent", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	handlePaymentsIntent(stripe, orders)(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestHandlePaymentsWebhook_MarksPaid(t *testing.T) {
	secret := "whsec_test"
	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	putTestOrder(t, orders, "ord_abc", "andreas@example.com", "card", 8990, 0)

	payload := []byte(`{"id":"evt_1","type":"payment_intent.succeeded","data":{"object":{"id":"pi_123","object":"payment_intent","status":"succeeded","amount":8990,"amount_received":8990,"metadata":{"orderId":"ord_abc"}}}}`)
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", "t="+strconv.FormatInt(ts, 10)+",v1="+sig)
	rr := httptest.NewRecorder()
	handlePaymentsWebhook(paymentsConfig{WebhookSecret: secret}, webhookDeps{Orders: orders})(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	o, ok, err := orders.get(context.Background(), "ord_abc")
	if err != nil || !ok {
		t.Fatalf("get order: err=%v ok=%v", err, ok)
	}
	if o.Status != "paid" {
		t.Fatalf("expected paid, got %q", o.Status)
	}
}

func TestHandlePaymentsWebhook_InvalidSignature(t *testing.T) {
	orders, _, cleanup := newTestStore(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Stripe-Signature", "t=1,v1=bogus")
	rr := httptest.NewRecorder()
	handlePaymentsWebhook(paymentsConfig{WebhookSecret: "whsec_x"}, webhookDeps{Orders: orders})(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}
