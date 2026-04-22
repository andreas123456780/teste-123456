package main

// Melhor Envio integration.
//
// Exposes three public-facing concerns for the NAST store:
//
//	POST /api/shipping/quote         → calculate shipping options for a given
//	                                    zip code and cart (public, rate limited)
//	POST /api/shipping/label         → purchase and generate a shipping label
//	                                    (admin only, gated by ADMIN_TOKEN)
//	GET  /api/shipping/track/:id     → fetch tracking status for an ME order id
//	                                    (admin only)
//
// All outbound calls go through shippingClient, which is environment-aware
// (sandbox vs production) and injects the token + user agent that Melhor
// Envio requires. The client is written against the stdlib only so we keep
// the "zero external deps" posture of the rest of the backend.

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ----- Configuration -----

const (
	meBaseSandbox = "https://sandbox.melhorenvio.com.br"
	meBaseProd    = "https://melhorenvio.com.br"

	// Conservative per-call timeout. ME calculate typically returns in
	// <800ms; give generous headroom but fail fast rather than hang.
	meHTTPTimeout = 8 * time.Second
)

// shippingConfig is hydrated from environment variables at startup. If the
// token is empty the handlers respond with 503 so the frontend can degrade
// gracefully (fall back to "combinar frete por WhatsApp").
type shippingConfig struct {
	BaseURL     string
	AccessToken string
	UserAgent   string // ME requires `App (contact@email)` format
	OriginZip   string // e.g. "01310100"
	AdminToken  string // for /label and /track endpoints
}

func loadShippingConfig() shippingConfig {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("MELHOR_ENVIO_ENV")))
	base := meBaseSandbox
	token := os.Getenv("MELHOR_ENVIO_ACCESS_TOKEN_SANDBOX")
	if env == "production" || env == "prod" {
		base = meBaseProd
		token = os.Getenv("MELHOR_ENVIO_ACCESS_TOKEN")
	}
	ua := strings.TrimSpace(os.Getenv("MELHOR_ENVIO_USER_AGENT"))
	if ua == "" {
		ua = "NAST Streetwear (contato@nast.example)"
	}
	origin := digitsOnly(os.Getenv("MELHOR_ENVIO_ORIGIN_ZIP"))
	return shippingConfig{
		BaseURL:     base,
		AccessToken: strings.TrimSpace(token),
		UserAgent:   ua,
		OriginZip:   origin,
		AdminToken:  strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
	}
}

// ----- Domain types -----

// ShippingPackage mirrors the subset of the ME "products" payload we need.
// Dimensions in cm, weight in kg, insurance_value in BRL (float).
type ShippingPackage struct {
	ID             string  `json:"id"`
	Width          float64 `json:"width"`
	Height         float64 `json:"height"`
	Length         float64 `json:"length"`
	Weight         float64 `json:"weight"`
	InsuranceValue float64 `json:"insurance_value"`
	Quantity       int     `json:"quantity"`
}

type shippingAddress struct {
	PostalCode string `json:"postal_code"`
}

type shippingOptions struct {
	Receipt  bool `json:"receipt"`
	OwnHand  bool `json:"own_hand"`
	Reverse  bool `json:"reverse"`
	NonComm  bool `json:"non_commercial"`
	Insurance bool `json:"insurance_value,omitempty"`
}

// meCalculateRequest matches POST /api/v2/me/shipment/calculate.
type meCalculateRequest struct {
	From     shippingAddress   `json:"from"`
	To       shippingAddress   `json:"to"`
	Products []ShippingPackage `json:"products"`
	Options  shippingOptions   `json:"options"`
	Services string            `json:"services,omitempty"`
}

// meQuote is a single line returned by ME's calculate endpoint. Some
// carriers return an `error` string instead of prices — we surface those
// so the UI can show a reason when a carrier is unavailable.
type meQuote struct {
	ID            int             `json:"id"`
	Name          string          `json:"name"`
	Price         json.RawMessage `json:"price"`         // sometimes string, sometimes number
	CustomPrice   json.RawMessage `json:"custom_price"`  // same
	DeliveryTime  int             `json:"delivery_time"` // business days
	DeliveryRange struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"delivery_range"`
	CustomDeliveryRange struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"custom_delivery_range"`
	Company struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	} `json:"company"`
	Error string `json:"error,omitempty"`
}

// ShippingQuoteRequest is what the frontend sends to us.
type ShippingQuoteRequest struct {
	ZipCode string            `json:"zipCode"`
	Items   []ShippingCartRef `json:"items"`
}

type ShippingCartRef struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

// ShippingQuoteOption is the normalized shape we return to the frontend.
type ShippingQuoteOption struct {
	ServiceID    int     `json:"serviceId"`
	CompanyID    int     `json:"companyId"`
	CompanyName  string  `json:"companyName"`
	ServiceName  string  `json:"serviceName"`
	PriceCents   int     `json:"priceCents"`
	DeliveryMin  int     `json:"deliveryMinDays"`
	DeliveryMax  int     `json:"deliveryMaxDays"`
	Error        string  `json:"error,omitempty"`
}

// ----- Client -----

type shippingClient struct {
	cfg    shippingConfig
	http   *http.Client
}

func newShippingClient(cfg shippingConfig) *shippingClient {
	return &shippingClient{
		cfg:  cfg,
		http: &http.Client{Timeout: meHTTPTimeout},
	}
}

// call issues an authenticated JSON request against ME. On non-2xx the
// response body is returned in the error so callers can log ME's reason
// (without leaking the token).
func (c *shippingClient) call(ctx context.Context, method, path string, body any, out any) error {
	if c.cfg.AccessToken == "" {
		return errors.New("melhor envio token not configured")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode ME request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build ME request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call ME: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MiB cap
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Trim the body to keep server logs readable.
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 512 {
			snippet = snippet[:512] + "…"
		}
		return fmt.Errorf("ME %s %s: %d %s", method, path, resp.StatusCode, snippet)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode ME response: %w", err)
	}
	return nil
}

// Calculate asks ME for quotes for every eligible carrier/service. It is
// intentionally forgiving: individual carrier failures come back as quotes
// with `Error` populated — we hand those through instead of failing the
// whole request.
func (c *shippingClient) Calculate(ctx context.Context, fromZip, toZip string, products []ShippingPackage) ([]meQuote, error) {
	req := meCalculateRequest{
		From:     shippingAddress{PostalCode: digitsOnly(fromZip)},
		To:       shippingAddress{PostalCode: digitsOnly(toZip)},
		Products: products,
	}
	var quotes []meQuote
	if err := c.call(ctx, http.MethodPost, "/api/v2/me/shipment/calculate", req, &quotes); err != nil {
		return nil, err
	}
	return quotes, nil
}

// AddToCart adds a shipment to the authenticated ME cart. Returns the ME
// order id which is later passed to Checkout + Generate + Track.
func (c *shippingClient) AddToCart(ctx context.Context, body map[string]any) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/v2/me/cart", body, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("ME cart response missing id")
	}
	return out.ID, nil
}

// Checkout purchases one or more ME orders from the authenticated cart. It
// debits the user's ME wallet, which must have sufficient balance.
func (c *shippingClient) Checkout(ctx context.Context, orderIDs []string) (json.RawMessage, error) {
	var out json.RawMessage
	body := map[string]any{"orders": orderIDs}
	if err := c.call(ctx, http.MethodPost, "/api/v2/me/shipment/checkout", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Generate triggers PDF generation for purchased labels.
func (c *shippingClient) Generate(ctx context.Context, orderIDs []string) (json.RawMessage, error) {
	var out json.RawMessage
	body := map[string]any{"orders": orderIDs}
	if err := c.call(ctx, http.MethodPost, "/api/v2/me/shipment/generate", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Print returns the link the admin can open to print generated labels.
func (c *shippingClient) Print(ctx context.Context, orderIDs []string, mode string) (json.RawMessage, error) {
	var out json.RawMessage
	body := map[string]any{"orders": orderIDs, "mode": mode}
	if err := c.call(ctx, http.MethodPost, "/api/v2/me/shipment/print", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Tracking returns current status per ME order id.
func (c *shippingClient) Tracking(ctx context.Context, orderIDs []string) (json.RawMessage, error) {
	var out json.RawMessage
	body := map[string]any{"orders": orderIDs}
	if err := c.call(ctx, http.MethodPost, "/api/v2/me/shipment/tracking", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ----- Quote cache -----

// Quote cache avoids hammering ME with the same (origin, dest, cart) tuple.
// Uses a short TTL because rates change and ME has its own rate limits.
type quoteCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	items map[string]quoteCacheEntry
}

type quoteCacheEntry struct {
	at     time.Time
	quotes []ShippingQuoteOption
}

func newQuoteCache(ttl time.Duration) *quoteCache {
	c := &quoteCache{ttl: ttl, items: make(map[string]quoteCacheEntry)}
	if ttl > 0 {
		go c.gc()
	}
	return c
}

// gc evicts expired entries on a fixed cadence so the cache map can't grow
// unbounded with stale keys that the reader path never revisits.
func (c *quoteCache) gc() {
	interval := c.ttl
	if interval < time.Minute {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		c.mu.Lock()
		now := time.Now()
		for k, e := range c.items {
			if now.Sub(e.at) > c.ttl {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}

func (c *quoteCache) get(key string) ([]ShippingQuoteOption, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok || time.Since(e.at) > c.ttl {
		return nil, false
	}
	return e.quotes, true
}

func (c *quoteCache) put(key string, quotes []ShippingQuoteOption) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = quoteCacheEntry{at: time.Now(), quotes: quotes}
}

// ----- Product dimension presets -----
//
// Until we pesar as peças reais, every item in the NAST catalog ships in a
// standard polybag. If a product isn't in this map we fall back to a
// conservative 250g / 30x25x3cm envelope.
var packagingPresets = map[string]ShippingPackage{
	"p-tee-bw-black": {Width: 25, Height: 3, Length: 30, Weight: 0.22, InsuranceValue: 89.90},
	"p-tee-bw-white": {Width: 25, Height: 3, Length: 30, Weight: 0.22, InsuranceValue: 89.90},
	"p-boxy-black":   {Width: 28, Height: 4, Length: 32, Weight: 0.28, InsuranceValue: 99.90},
	"p-boxy-white":   {Width: 28, Height: 4, Length: 32, Weight: 0.28, InsuranceValue: 99.90},
}

func packagingFor(productID string) ShippingPackage {
	if p, ok := packagingPresets[productID]; ok {
		return p
	}
	return ShippingPackage{Width: 25, Height: 3, Length: 30, Weight: 0.25, InsuranceValue: 0}
}

// buildPackages converts cart refs → ME "products" payload. Consolidates by
// product id so carriers see the right quantity.
func buildPackages(items []ShippingCartRef) ([]ShippingPackage, error) {
	if len(items) == 0 || len(items) > 50 {
		return nil, errors.New("invalid item count")
	}
	merged := make(map[string]*ShippingPackage, len(items))
	order := make([]string, 0, len(items))
	for _, it := range items {
		if it.Quantity <= 0 || it.Quantity > 20 {
			return nil, errors.New("invalid item quantity")
		}
		if it.ProductID == "" {
			return nil, errors.New("invalid product id")
		}
		if p, ok := merged[it.ProductID]; ok {
			p.Quantity += it.Quantity
			continue
		}
		pkg := packagingFor(it.ProductID)
		pkg.ID = it.ProductID
		pkg.Quantity = it.Quantity
		merged[it.ProductID] = &pkg
		order = append(order, it.ProductID)
	}
	out := make([]ShippingPackage, 0, len(order))
	for _, id := range order {
		out = append(out, *merged[id])
	}
	return out, nil
}

// ----- Quote normalizer -----

func normalizeQuotes(raw []meQuote) []ShippingQuoteOption {
	out := make([]ShippingQuoteOption, 0, len(raw))
	for _, q := range raw {
		opt := ShippingQuoteOption{
			ServiceID:   q.ID,
			CompanyID:   q.Company.ID,
			CompanyName: q.Company.Name,
			ServiceName: q.Name,
			DeliveryMin: q.DeliveryRange.Min,
			DeliveryMax: q.DeliveryRange.Max,
			Error:       q.Error,
		}
		// Prefer the seller's custom (possibly longer) promise when they set one.
		if q.CustomDeliveryRange.Min > 0 {
			opt.DeliveryMin = q.CustomDeliveryRange.Min
		}
		if q.CustomDeliveryRange.Max > 0 {
			opt.DeliveryMax = q.CustomDeliveryRange.Max
		}
		if opt.DeliveryMin == 0 && opt.DeliveryMax == 0 && q.DeliveryTime > 0 {
			opt.DeliveryMin = q.DeliveryTime
			opt.DeliveryMax = q.DeliveryTime
		}
		if q.Error == "" {
			cents, ok := parseBRLCents(q.Price)
			if !ok {
				// Some carriers return price as string with custom_price fallback.
				cents, ok = parseBRLCents(q.CustomPrice)
			}
			if ok {
				opt.PriceCents = cents
			}
		}
		out = append(out, opt)
	}
	return out
}

// parseBRLCents reads ME's price field (which is sometimes a JSON number and
// sometimes a string like "12.34") and returns integer cents.
func parseBRLCents(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0, false
	}
	// Unquote if it's a JSON string.
	if strings.HasPrefix(s, "\"") {
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return 0, false
		}
		s = strings.TrimSpace(str)
	}
	// Normalize comma decimals just in case.
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, false
	}
	// Manual parse to avoid floating-point rounding on cents.
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	intPart, fracPart, _ := strings.Cut(s, ".")
	var cents int
	for _, r := range intPart {
		if r < '0' || r > '9' {
			return 0, false
		}
		cents = cents*10 + int(r-'0')
	}
	cents *= 100
	// Take up to 2 fractional digits, pad with zero if needed.
	fracDigits := 0
	for _, r := range fracPart {
		if r < '0' || r > '9' {
			return 0, false
		}
		if fracDigits == 0 {
			cents += int(r-'0') * 10
		} else if fracDigits == 1 {
			cents += int(r - '0')
		}
		fracDigits++
		if fracDigits == 2 {
			break
		}
	}
	if neg {
		cents = -cents
	}
	return cents, true
}

// digitsOnly strips everything but digits (useful for CEPs "01310-100" → "01310100").
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validBrazilianZip(cep string) bool {
	d := digitsOnly(cep)
	return len(d) == 8
}

// ----- HTTP handlers -----

func handleShippingQuote(c *shippingClient, cache *quoteCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if c.cfg.AccessToken == "" || c.cfg.OriginZip == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shipping unavailable"})
			return
		}
		var req ShippingQuoteRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if !validBrazilianZip(req.ZipCode) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid zip"})
			return
		}
		pkgs, err := buildPackages(req.Items)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		destZip := digitsOnly(req.ZipCode)
		key := cacheKey(c.cfg.OriginZip, destZip, pkgs)
		if hit, ok := cache.get(key); ok {
			writeJSON(w, http.StatusOK, map[string]any{"options": hit, "cached": true})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), meHTTPTimeout)
		defer cancel()
		raw, err := c.Calculate(ctx, c.cfg.OriginZip, destZip, pkgs)
		if err != nil {
			log.Printf("shipping.quote error: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "shipping upstream failed"})
			return
		}
		opts := normalizeQuotes(raw)
		cache.put(key, opts)
		writeJSON(w, http.StatusOK, map[string]any{"options": opts, "cached": false})
	}
}

// requireAdmin checks X-Admin-Token against ADMIN_TOKEN in constant time.
// Returns true if the request is authorized. Writes 401 otherwise.
func requireAdmin(adminToken string, w http.ResponseWriter, r *http.Request) bool {
	if adminToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin disabled"})
		return false
	}
	got := r.Header.Get("X-Admin-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(adminToken)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}

type shippingLabelRequest struct {
	Cart map[string]any `json:"cart"`
}

func handleShippingLabel(c *shippingClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !requireAdmin(c.cfg.AdminToken, w, r) {
			return
		}
		if c.cfg.AccessToken == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shipping unavailable"})
			return
		}
		var req shippingLabelRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil || req.Cart == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cart"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*meHTTPTimeout)
		defer cancel()
		orderID, err := c.AddToCart(ctx, req.Cart)
		if err != nil {
			log.Printf("shipping.cart error: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "cart failed"})
			return
		}
		checkout, err := c.Checkout(ctx, []string{orderID})
		if err != nil {
			log.Printf("shipping.checkout error: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "checkout failed", "orderId": orderID})
			return
		}
		generated, err := c.Generate(ctx, []string{orderID})
		if err != nil {
			log.Printf("shipping.generate error: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "generate failed", "orderId": orderID})
			return
		}
		printed, err := c.Print(ctx, []string{orderID}, "private")
		if err != nil {
			log.Printf("shipping.print error: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "print failed", "orderId": orderID})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"orderId":  orderID,
			"checkout": checkout,
			"generate": generated,
			"print":    printed,
		})
	}
}

func handleShippingTrack(c *shippingClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !requireAdmin(c.cfg.AdminToken, w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/shipping/track/")
		if id == "" || strings.ContainsAny(id, "/?#") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), meHTTPTimeout)
		defer cancel()
		tr, err := c.Tracking(ctx, []string{id})
		if err != nil {
			log.Printf("shipping.track error: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "tracking upstream failed"})
			return
		}
		writeJSON(w, http.StatusOK, tr)
	}
}

// cacheKey builds a stable key from origin + dest + merged cart so
// equivalent requests hit the cache.
func cacheKey(from, to string, pkgs []ShippingPackage) string {
	var b strings.Builder
	b.WriteString(from)
	b.WriteByte('|')
	b.WriteString(to)
	for _, p := range pkgs {
		fmt.Fprintf(&b, "|%s:%d", p.ID, p.Quantity)
	}
	return b.String()
}
