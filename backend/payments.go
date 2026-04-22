package main

// Stripe payments integration.
//
// Exposes two endpoints:
//
//	POST /api/payments/intent   → creates a PaymentIntent for a previously
//	                              created (pending) order and returns the
//	                              Stripe client secret the browser uses to
//	                              confirm payment.
//	POST /api/payments/webhook  → receives Stripe events (signed with
//	                              STRIPE_WEBHOOK_SECRET) and marks the
//	                              underlying order as `paid` when a
//	                              payment_intent.succeeded is observed.
//
// The integration speaks the Stripe REST API directly over net/http — no
// SDK — to keep the zero-external-deps posture of the backend.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	stripeAPIBase      = "https://api.stripe.com"
	stripeHTTPTimeout  = 10 * time.Second
	stripeCurrencyBRL  = "brl"
	stripeSigTolerance = 5 * time.Minute
)

// stripeAPIBaseOverride lets tests redirect outbound calls at a fake
// server. Left empty in production.
var stripeAPIBaseOverride = ""

func stripeBaseURL() string {
	if stripeAPIBaseOverride != "" {
		return stripeAPIBaseOverride
	}
	return stripeAPIBase
}

// paymentsConfig is hydrated from environment variables at startup.
type paymentsConfig struct {
	SecretKey     string
	WebhookSecret string
}

func loadPaymentsConfig() paymentsConfig {
	return paymentsConfig{
		SecretKey:     strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")),
		WebhookSecret: strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
	}
}

// ----- Stripe HTTP client -----

type stripeClient struct {
	cfg  paymentsConfig
	http *http.Client
}

func newStripeClient(cfg paymentsConfig) *stripeClient {
	return &stripeClient{
		cfg:  cfg,
		http: &http.Client{Timeout: stripeHTTPTimeout},
	}
}

// postForm issues a form-urlencoded POST to Stripe. Stripe's API takes
// flat form bodies (with bracketed keys for nested objects), so we build
// url.Values directly.
func (c *stripeClient) postForm(ctx context.Context, path string, form url.Values, out any) error {
	if c.cfg.SecretKey == "" {
		return errors.New("stripe secret key not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stripeBaseURL()+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build stripe request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+c.cfg.SecretKey)
	req.Header.Set("Stripe-Version", "2024-06-20")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call stripe: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MiB cap
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 512 {
			snippet = snippet[:512] + "…"
		}
		return fmt.Errorf("stripe %s: %d %s", path, resp.StatusCode, snippet)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode stripe response: %w", err)
	}
	return nil
}

// stripePaymentIntent is the trimmed subset of the Stripe PaymentIntent
// resource we care about.
type stripePaymentIntent struct {
	ID           string `json:"id"`
	ClientSecret string `json:"client_secret"`
	Status       string `json:"status"`
	Amount       int    `json:"amount"`
	Currency     string `json:"currency"`
}

// CreatePaymentIntent creates a Stripe PaymentIntent with the given amount
// (in cents) and payment method. Accepted methods: "card" or "pix".
// orderID is attached as metadata so the webhook can look the order up.
func (c *stripeClient) CreatePaymentIntent(ctx context.Context, amountCents int, method, orderID, receiptEmail string) (*stripePaymentIntent, error) {
	if amountCents <= 0 {
		return nil, errors.New("invalid amount")
	}
	form := url.Values{}
	form.Set("amount", strconv.Itoa(amountCents))
	form.Set("currency", stripeCurrencyBRL)
	form.Add("payment_method_types[]", method)
	form.Set("metadata[orderId]", orderID)
	// Non-sensitive description; shows up on Stripe dashboard.
	form.Set("description", "NAST order "+orderID)
	if receiptEmail != "" {
		form.Set("receipt_email", receiptEmail)
	}
	var pi stripePaymentIntent
	if err := c.postForm(ctx, "/v1/payment_intents", form, &pi); err != nil {
		return nil, err
	}
	return &pi, nil
}

// ----- Order store (in-memory) -----
//
// This is an MVP store: pending orders live only in RAM and are evicted on
// restart. That's fine for sandbox testing but should be replaced with a
// real database before going live.

type pendingOrder struct {
	ID              string
	TotalCents      int
	PaymentMethod   string
	PaymentIntentID string
	Status          string // "pending_payment" | "paid" | "failed"
	CreatedAt       time.Time
	Email           string
	ShippingCents   int
	ShippingSvcID   int
	ShippingSvcName string
}

type orderStore struct {
	mu     sync.RWMutex
	byID   map[string]*pendingOrder
	byPI   map[string]string // payment_intent id → order id
	maxAge time.Duration
}

func newOrderStore() *orderStore {
	s := &orderStore{
		byID:   make(map[string]*pendingOrder),
		byPI:   make(map[string]string),
		maxAge: 24 * time.Hour,
	}
	go s.gc()
	return s
}

func (s *orderStore) gc() {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		now := time.Now()
		for id, o := range s.byID {
			if now.Sub(o.CreatedAt) > s.maxAge {
				delete(s.byID, id)
				if o.PaymentIntentID != "" {
					delete(s.byPI, o.PaymentIntentID)
				}
			}
		}
		s.mu.Unlock()
	}
}

func (s *orderStore) put(o *pendingOrder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[o.ID] = o
	if o.PaymentIntentID != "" {
		s.byPI[o.PaymentIntentID] = o.ID
	}
}

func (s *orderStore) get(id string) (*pendingOrder, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, ok := s.byID[id]
	return o, ok
}

func (s *orderStore) byPaymentIntent(piID string) (*pendingOrder, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byPI[piID]
	if !ok {
		return nil, false
	}
	o, ok := s.byID[id]
	return o, ok
}

// ----- Public HTTP handlers -----

type paymentIntentRequest struct {
	OrderID string `json:"orderId"`
}

type paymentIntentResponse struct {
	ClientSecret string `json:"clientSecret"`
	OrderID      string `json:"orderId"`
	Status       string `json:"status"`
	AmountCents  int    `json:"amountCents"`
	Method       string `json:"method"`
}

// handlePaymentsIntent attaches (or recreates, on-demand) a PaymentIntent
// for a pending order and returns the client secret the browser uses to
// confirm payment with Stripe Elements.
//
// We intentionally do NOT create the PaymentIntent inside /api/checkout:
// if the user closes the Elements UI and retries, we can re-hit this
// endpoint without creating orphan Stripe records.
func handlePaymentsIntent(stripe *stripeClient, orders *orderStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if stripe.cfg.SecretKey == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "payments unavailable"})
			return
		}
		var req paymentIntentRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		id := strings.TrimSpace(req.OrderID)
		if id == "" || !strings.HasPrefix(id, "ord_") || len(id) > 64 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid orderId"})
			return
		}
		o, ok := orders.get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
			return
		}
		if o.Status == "paid" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "order already paid"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), stripeHTTPTimeout)
		defer cancel()
		pi, err := stripe.CreatePaymentIntent(ctx, o.TotalCents+o.ShippingCents, o.PaymentMethod, o.ID, o.Email)
		if err != nil {
			log.Printf("payments.intent error: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment provider failed"})
			return
		}
		// Persist the PaymentIntent id so the webhook can resolve it back.
		o.PaymentIntentID = pi.ID
		orders.put(o)
		writeJSON(w, http.StatusOK, paymentIntentResponse{
			ClientSecret: pi.ClientSecret,
			OrderID:      o.ID,
			Status:       o.Status,
			AmountCents:  o.TotalCents + o.ShippingCents,
			Method:       o.PaymentMethod,
		})
	}
}

// ----- Webhook -----

// stripeEvent is the minimal shape we decode out of an incoming Stripe
// event. We intentionally avoid strongly typing the data object beyond
// `payment_intent` — anything else we ignore.
type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object struct {
			ID             string `json:"id"`
			Object         string `json:"object"`
			Status         string `json:"status"`
			Amount         int    `json:"amount"`
			AmountReceived int    `json:"amount_received"`
			Metadata       struct {
				OrderID string `json:"orderId"`
			} `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

// verifyStripeSignature implements the Stripe webhook signing protocol:
// header value is `t=<timestamp>,v1=<hex-hmac>,v1=<hex-hmac>...` and
// signed payload is `<timestamp>.<raw-body>`.
func verifyStripeSignature(secret string, header string, payload []byte, now time.Time) error {
	if secret == "" {
		return errors.New("webhook secret not configured")
	}
	if header == "" {
		return errors.New("missing Stripe-Signature header")
	}
	var tsStr string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			tsStr = kv[1]
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if tsStr == "" || len(sigs) == 0 {
		return errors.New("malformed signature header")
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	// Reject obviously old/future timestamps to limit replay.
	if d := now.Sub(time.Unix(ts, 0)); d < -stripeSigTolerance || d > stripeSigTolerance {
		return fmt.Errorf("timestamp outside tolerance (%v)", d)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	for _, s := range sigs {
		if subtle.ConstantTimeCompare([]byte(s), []byte(expected)) == 1 {
			return nil
		}
	}
	return errors.New("no matching signature")
}

// handlePaymentsWebhook validates the Stripe signature and, for
// payment_intent events we care about, updates the corresponding order
// status. Always responds 200 after a verified delivery so Stripe does
// not retry — unless the signature itself failed.
func handlePaymentsWebhook(cfg paymentsConfig, orders *orderStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		// Cap the raw body; Stripe events are well under 256KiB.
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<18))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
			return
		}
		if err := verifyStripeSignature(cfg.WebhookSecret, r.Header.Get("Stripe-Signature"), raw, time.Now()); err != nil {
			log.Printf("payments.webhook signature error: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid signature"})
			return
		}
		var ev stripeEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		switch ev.Type {
		case "payment_intent.succeeded":
			applyOrderStatus(orders, ev, "paid")
		case "payment_intent.payment_failed", "payment_intent.canceled":
			applyOrderStatus(orders, ev, "failed")
		default:
			// Ignore other events; still ack to stop retries.
		}
		writeJSON(w, http.StatusOK, map[string]string{"received": "ok"})
	}
}

// applyOrderStatus looks up the order either by metadata.orderId or by the
// payment_intent id and updates its status. No-op if the order isn't
// known — most commonly because the in-memory store was restarted.
func applyOrderStatus(orders *orderStore, ev stripeEvent, status string) {
	obj := ev.Data.Object
	if orderID := strings.TrimSpace(obj.Metadata.OrderID); orderID != "" {
		if o, ok := orders.get(orderID); ok {
			o.Status = status
			if o.PaymentIntentID == "" {
				o.PaymentIntentID = obj.ID
			}
			orders.put(o)
			log.Printf("payments.webhook: order %s → %s via metadata (pi=%s)", o.ID, status, obj.ID)
			return
		}
	}
	if o, ok := orders.byPaymentIntent(obj.ID); ok {
		o.Status = status
		orders.put(o)
		log.Printf("payments.webhook: order %s → %s via pi lookup (pi=%s)", o.ID, status, obj.ID)
		return
	}
	log.Printf("payments.webhook: no local order for pi=%s (type=%s) — ignoring", obj.ID, ev.Type)
}
