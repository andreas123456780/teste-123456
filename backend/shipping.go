package main

// SuperFrete integration.
//
// Exposes three public-facing concerns for the NAST store:
//
//	POST /api/shipping/quote         → calculate shipping options for a given
//	                                    zip code and cart (public, rate limited)
//	POST /api/shipping/label         → purchase and generate a shipping label
//	                                    (admin only, gated by ADMIN_TOKEN)
//	GET  /api/shipping/track/:id     → fetch tracking info for a shipment id
//	                                    (admin only)
//
// All outbound calls go through shippingClient, which is environment-aware
// (sandbox vs production) and injects the token + user agent that
// SuperFrete requires. The client is written against the stdlib only so we
// keep the "zero external deps" posture of the rest of the backend.

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
	superfreteBaseSandbox = "https://sandbox.superfrete.com"
	superfreteBaseProd    = "https://api.superfrete.com"

	// Conservative per-call timeout. SuperFrete /calculator typically
	// returns in <1s; give generous headroom but fail fast rather than
	// hang.
	superfreteHTTPTimeout = 10 * time.Second

	superfretePlatform = "NAST Streetwear"
)

// shippingConfig is hydrated from environment variables at startup. If the
// token is empty the handlers respond with 503 so the frontend can degrade
// gracefully (fall back to "combinar frete por WhatsApp").
type shippingConfig struct {
	BaseURL     string
	AccessToken string
	UserAgent   string // SuperFrete requires `App (contact@email)` format
	OriginZip   string // e.g. "08503000"
	AdminToken  string // for /label and /track endpoints
	From        senderAddress
	// Autopay controls whether the label pipeline debits the SuperFrete
	// wallet immediately after adding the order to the cart. When false
	// (default), runLabelJob stops at AddToCart and leaves the order in
	// "awaiting_shipment" so the operator can pay manually via the
	// SuperFrete app (cheaper Pix, coupons, etc.). When true, the old
	// behaviour is preserved: checkout → generate → print synchronously.
	Autopay bool
}

// senderAddress holds the seller's address. SuperFrete's /api/v0/cart
// endpoint requires a populated `from` object so the label can be printed
// with the correct return address.
type senderAddress struct {
	Name     string
	Address  string
	District string
	City     string
	State    string // 2-letter UF, e.g. "SP"
	Document string
	Phone    string
	Email    string
	Company  string
}

func (s senderAddress) complete() bool {
	return s.Name != "" && s.Address != "" && s.District != "" &&
		s.City != "" && s.State != ""
}

func loadShippingConfig() shippingConfig {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("SUPERFRETE_ENV")))
	base := superfreteBaseSandbox
	if env == "production" || env == "prod" {
		base = superfreteBaseProd
	}
	token := strings.TrimSpace(os.Getenv("SUPERFRETE_TOKEN"))
	ua := strings.TrimSpace(os.Getenv("SUPERFRETE_USER_AGENT"))
	if ua == "" {
		ua = "NAST Streetwear (contato@nast.example)"
	}
	origin := digitsOnly(os.Getenv("SUPERFRETE_ORIGIN_ZIP"))
	return shippingConfig{
		BaseURL:     base,
		AccessToken: token,
		UserAgent:   ua,
		OriginZip:   origin,
		AdminToken:  strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		From: senderAddress{
			Name:     strings.TrimSpace(os.Getenv("SUPERFRETE_FROM_NAME")),
			Address:  strings.TrimSpace(os.Getenv("SUPERFRETE_FROM_ADDRESS")),
			District: strings.TrimSpace(os.Getenv("SUPERFRETE_FROM_DISTRICT")),
			City:     strings.TrimSpace(os.Getenv("SUPERFRETE_FROM_CITY")),
			State:    strings.ToUpper(strings.TrimSpace(os.Getenv("SUPERFRETE_FROM_STATE"))),
			Document: digitsOnly(os.Getenv("SUPERFRETE_FROM_DOCUMENT")),
			Phone:    strings.TrimSpace(os.Getenv("SUPERFRETE_FROM_PHONE")),
			Email:    strings.TrimSpace(os.Getenv("SUPERFRETE_FROM_EMAIL")),
			Company:  strings.TrimSpace(os.Getenv("SUPERFRETE_FROM_COMPANY")),
		},
		Autopay: isTruthy(os.Getenv("SUPERFRETE_AUTOPAY")),
	}
}

// isTruthy accepts any of the common bool-ish env var spellings.
// Returns false for unset/empty strings so the default stays "cart
// only" (no wallet debit).
func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return true
	}
	return false
}

// ----- Domain types -----

// ShippingPackage is our internal per-item packaging record. It ends up
// being aggregated into a single `package` envelope on the wire because
// SuperFrete's calculator expects one combined box.
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

// sfVolume mirrors the `package` field SuperFrete expects — a single merged
// box with total dimensions and weight.
type sfVolume struct {
	Height float64 `json:"height"`
	Width  float64 `json:"width"`
	Length float64 `json:"length"`
	Weight float64 `json:"weight"`
}

type sfCalculatorRequest struct {
	From     shippingAddress `json:"from"`
	To       shippingAddress `json:"to"`
	Package  sfVolume        `json:"package"`
	Services string          `json:"services,omitempty"`
	Options  sfOptions       `json:"options"`
}

type sfOptions struct {
	OwnHand           bool    `json:"own_hand"`
	Receipt           bool    `json:"receipt"`
	InsuranceValue    float64 `json:"insurance_value"`
	UseInsuranceValue bool    `json:"use_insurance_value"`
}

// sfQuote is a single line returned by SuperFrete's calculator endpoint.
// Some carriers return an `error` string instead of prices — we surface
// those so the UI can show a reason when a carrier is unavailable.
type sfQuote struct {
	ID           int             `json:"id"`
	Name         string          `json:"name"`
	Price        json.RawMessage `json:"price"`         // sometimes string, sometimes number
	CustomPrice  json.RawMessage `json:"custom_price"`  // same
	Discount     json.RawMessage `json:"discount"`      // discount applied
	DeliveryTime int             `json:"delivery_time"` // business days
	DeliveryRange struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"delivery_range"`
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
// Kept stable across the Melhor Envio → SuperFrete swap so the React
// client doesn't need to branch.
type ShippingQuoteOption struct {
	ServiceID   int    `json:"serviceId"`
	CompanyID   int    `json:"companyId"`
	CompanyName string `json:"companyName"`
	ServiceName string `json:"serviceName"`
	PriceCents  int    `json:"priceCents"`
	DeliveryMin int    `json:"deliveryMinDays"`
	DeliveryMax int    `json:"deliveryMaxDays"`
	Error       string `json:"error,omitempty"`
}

// ----- Client -----

type shippingClient struct {
	cfg  shippingConfig
	http *http.Client
}

func newShippingClient(cfg shippingConfig) *shippingClient {
	return &shippingClient{
		cfg:  cfg,
		http: &http.Client{Timeout: superfreteHTTPTimeout},
	}
}

// call issues an authenticated JSON request against SuperFrete. On non-2xx
// the response body is returned in the error so callers can log the reason
// (without leaking the token).
func (c *shippingClient) call(ctx context.Context, method, path string, body any, out any) error {
	if c.cfg.AccessToken == "" {
		return errors.New("superfrete token not configured")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode superfrete request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build superfrete request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call superfrete: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MiB cap
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Trim the body to keep server logs readable.
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 512 {
			snippet = snippet[:512] + "…"
		}
		return fmt.Errorf("superfrete %s %s: %d %s", method, path, resp.StatusCode, snippet)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode superfrete response: %w", err)
	}
	return nil
}

// Calculate asks SuperFrete for quotes for every eligible carrier/service.
// It is intentionally forgiving: individual carrier failures come back as
// quotes with `Error` populated — we hand those through instead of failing
// the whole request.
func (c *shippingClient) Calculate(ctx context.Context, fromZip, toZip string, pkg sfVolume, insuranceValue float64) ([]sfQuote, error) {
	req := sfCalculatorRequest{
		From:    shippingAddress{PostalCode: digitsOnly(fromZip)},
		To:      shippingAddress{PostalCode: digitsOnly(toZip)},
		Package: pkg,
		// 1: PAC, 2: SEDEX, 17: Mini Envios — the standard trio for
		// small-parcel apparel.
		Services: "1,2,17",
		Options: sfOptions{
			InsuranceValue:    insuranceValue,
			UseInsuranceValue: insuranceValue > 0,
		},
	}
	var quotes []sfQuote
	if err := c.call(ctx, http.MethodPost, "/api/v0/calculator", req, &quotes); err != nil {
		return nil, err
	}
	return quotes, nil
}

// AddToCart adds a shipment to the authenticated SuperFrete cart. Returns
// the shipment id which is later passed to Checkout + Generate + Print +
// order/info.
func (c *shippingClient) AddToCart(ctx context.Context, body map[string]any) (string, error) {
	if _, ok := body["platform"]; !ok {
		body["platform"] = superfretePlatform
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/v0/cart", body, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("superfrete cart response missing id")
	}
	return out.ID, nil
}

// Checkout purchases one or more SuperFrete shipments from the
// authenticated cart. It debits the user's SuperFrete wallet, which must
// have sufficient balance.
func (c *shippingClient) Checkout(ctx context.Context, orderIDs []string) (json.RawMessage, error) {
	var out json.RawMessage
	body := map[string]any{"orders": orderIDs}
	if err := c.call(ctx, http.MethodPost, "/api/v0/checkout", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Generate triggers PDF generation for purchased labels.
func (c *shippingClient) Generate(ctx context.Context, orderIDs []string) (json.RawMessage, error) {
	var out json.RawMessage
	body := map[string]any{"orders": orderIDs}
	if err := c.call(ctx, http.MethodPost, "/api/v0/generate", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Print returns the link the admin can open to print generated labels.
func (c *shippingClient) Print(ctx context.Context, orderIDs []string, mode string) (json.RawMessage, error) {
	var out json.RawMessage
	body := map[string]any{"orders": orderIDs, "mode": mode}
	if err := c.call(ctx, http.MethodPost, "/api/v0/print", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// OrderInfo returns tracking code + status for a SuperFrete shipment id.
func (c *shippingClient) OrderInfo(ctx context.Context, orderID string) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.call(ctx, http.MethodGet, "/api/v0/order/info/"+orderID, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ----- Quote cache -----

// Quote cache avoids hammering SuperFrete with the same (origin, dest,
// cart) tuple. Uses a short TTL because rates change and SuperFrete has
// its own rate limits.
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
	"p-bb-look-black": {Width: 25, Height: 3, Length: 30, Weight: 0.18, InsuranceValue: 79.90},
	"p-bb-look-white": {Width: 25, Height: 3, Length: 30, Weight: 0.18, InsuranceValue: 79.90},
"p-secret-red":    {Width: 28, Height: 4, Length: 32, Weight: 0.28, InsuranceValue: 129.90},
}

func packagingFor(productID string) ShippingPackage {
	if p, ok := packagingPresets[productID]; ok {
		return p
	}
	return ShippingPackage{Width: 25, Height: 3, Length: 30, Weight: 0.25, InsuranceValue: 0}
}

// buildPackages converts cart refs → per-item packaging records. Consolidates
// by product id so downstream aggregation sees the right quantity.
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

// mergeVolumes consolidates per-item packaging records into a single
// shipment volume. Weight is summed across all units; the outer box
// dimensions are the max of each side (reflects "one polybag big enough to
// hold all pieces"). Total insurance value is summed separately.
func mergeVolumes(pkgs []ShippingPackage) (sfVolume, float64) {
	var vol sfVolume
	var insurance float64
	for _, p := range pkgs {
		q := float64(p.Quantity)
		if q <= 0 {
			q = 1
		}
		vol.Weight += p.Weight * q
		if p.Width > vol.Width {
			vol.Width = p.Width
		}
		if p.Height > vol.Height {
			vol.Height = p.Height
		}
		if p.Length > vol.Length {
			vol.Length = p.Length
		}
		insurance += p.InsuranceValue * q
	}
	return vol, insurance
}

// ----- Quote normalizer -----

func normalizeQuotes(raw []sfQuote) []ShippingQuoteOption {
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

// parseBRLCents reads a price field (which is sometimes a JSON number and
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
		ctx, cancel := context.WithTimeout(r.Context(), superfreteHTTPTimeout)
		defer cancel()
		vol, insurance := mergeVolumes(pkgs)
		raw, err := c.Calculate(ctx, c.cfg.OriginZip, destZip, vol, insurance)
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
		ctx, cancel := context.WithTimeout(r.Context(), 3*superfreteHTTPTimeout)
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
		ctx, cancel := context.WithTimeout(r.Context(), superfreteHTTPTimeout)
		defer cancel()
		tr, err := c.OrderInfo(ctx, id)
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
