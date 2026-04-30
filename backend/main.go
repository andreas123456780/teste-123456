// Package main implements the backend API for NAST — streetwear autoral.
// Powers product catalog, checkout and security-hardened serving for the
// store frontend.
//
// Design goals:
//   - Zero external dependencies (stdlib only) to minimize the supply-chain
//     attack surface.
//   - Defense-in-depth: strict security headers, an allowlist CORS policy,
//     a lightweight IP-based rate limiter, bounded request bodies, and
//     constant-time comparisons where secrets are involved.
//   - Conservative defaults: read/write/idle timeouts, no directory listing,
//     JSON-only responses.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ----- Domain types -----

type Product struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	PriceCents    int      `json:"priceCents"`
	PixPriceCents int      `json:"pixPriceCents"`
	Category      string   `json:"category"`
	Image         string   `json:"image"`
	BackImage     string   `json:"backImage"`
	Colors        []string `json:"colors"`
	Sizes         []string `json:"sizes"`
	Tags          []string `json:"tags"`
	Stock         int      `json:"stock"`
	// StockBySize carries the per-size inventory. The keys match the
	// labels in Sizes (e.g. "P", "M", "Baby Look"). Missing keys are
	// treated as 0 (sold out). A fully empty map means the product has
	// not been migrated yet — the storefront falls back to the legacy
	// total Stock column for backwards compat.
	StockBySize map[string]int `json:"stockBySize"`
}

type CartItem struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
	Size      string `json:"size"`
	Color     string `json:"color"`
}

type CheckoutRequest struct {
	Items         []CartItem        `json:"items"`
	Name          string            `json:"name"`
	Email         string            `json:"email"`
	Document      string            `json:"document"` // CPF (11) or CNPJ (14); non-digits ignored
	Address       string            `json:"address"`  // logradouro (rua/avenida)
	AddressNumber string            `json:"addressNumber"`
	AddressComplement string        `json:"addressComplement,omitempty"`
	District      string            `json:"district"` // bairro
	City          string            `json:"city"`
	State         string            `json:"state"` // UF, 2 letters
	ZipCode       string            `json:"zipCode"`
	PaymentMethod string            `json:"paymentMethod"`
	Shipping      *CheckoutShipping `json:"shipping,omitempty"`
	CouponCode    string            `json:"couponCode,omitempty"`
}

// CheckoutShipping is optional and carries the SuperFrete quote the user
// picked on the client. The amount is folded into the PaymentIntent so a
// single charge covers both items and freight.
type CheckoutShipping struct {
	ServiceID   int    `json:"serviceId"`
	ServiceName string `json:"serviceName"`
	PriceCents  int    `json:"priceCents"`
}

// CheckoutResponse is what the browser receives after submitting the cart.
// With Stripe integration the order starts as `pending_payment` — the
// browser must then exchange `orderId` for a Stripe `clientSecret` via
// /api/payments/intent and confirm payment with Stripe Elements.
// `orderToken` is an HMAC-signed handle the frontend uses to build the
// /pedido/:token status URL without exposing the raw orderId in links.
type CheckoutResponse struct {
	OrderID       string `json:"orderId"`
	OrderToken    string `json:"orderToken"`
	TotalCents    int    `json:"totalCents"`
	ShippingCents int    `json:"shippingCents"`
	AmountCents   int    `json:"amountCents"`
	DiscountCents int    `json:"discountCents,omitempty"`
	CouponCode    string `json:"couponCode,omitempty"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	PaymentMethod string `json:"paymentMethod"`
}

// ----- In-memory catalog (seeded on startup) -----

// NAST streetwear · edição limitada — 4 peças.
var catalog = []Product{
	{ID: "p-tee-bw-black", Name: "CAMISETA BLACK & WHITE", Description: "Camiseta preta em algodão 30.1 penteado com print cursivo frontal em branco. Corte regular, gola reforçada.", PriceCents: 8990, PixPriceCents: 8541, Category: "Camisetas", Image: "tee-cursive-black.jpeg", BackImage: "tee-cursive-black-back.jpeg", Colors: []string{"preto"}, Sizes: []string{"P", "M", "G", "Baby Look"}, Tags: []string{"edição limitada"}, Stock: 24, StockBySize: map[string]int{"P": 6, "M": 6, "G": 6, "Baby Look": 6}},
	{ID: "p-tee-bw-white", Name: "CAMISA BLACK & WHITE", Description: "Camiseta branca em algodão 30.1 penteado com print cursivo frontal em preto. Corte regular, gola reforçada.", PriceCents: 8990, PixPriceCents: 8541, Category: "Camisetas", Image: "tee-cursive-white.jpeg", BackImage: "tee-cursive-white-back.jpeg", Colors: []string{"branco"}, Sizes: []string{"P", "M", "G", "Baby Look"}, Tags: []string{"edição limitada"}, Stock: 24, StockBySize: map[string]int{"P": 6, "M": 6, "G": 6, "Baby Look": 6}},
	{ID: "p-boxy-black", Name: "CAMISA BOXY NAST PRETA", Description: "Camiseta boxy preta em algodão pesado 240g com modelagem oversized, ombro caído e etiqueta tecida NAST.", PriceCents: 9990, PixPriceCents: 9491, Category: "Boxy", Image: "boxy-black.jpeg", BackImage: "boxy-black-back.jpeg", Colors: []string{"preto"}, Sizes: []string{"P", "M", "G"}, Tags: []string{"boxy fit"}, Stock: 18, StockBySize: map[string]int{"P": 6, "M": 6, "G": 6}},
	{ID: "p-boxy-white", Name: "CAMISETA BOXY NAST BRANCA", Description: "Camiseta boxy branca em algodão pesado 240g com modelagem oversized, ombro caído e etiqueta tecida NAST.", PriceCents: 9990, PixPriceCents: 9491, Category: "Boxy", Image: "boxy-white.jpeg", BackImage: "boxy-white-back.jpeg", Colors: []string{"branco"}, Sizes: []string{"P", "M", "G"}, Tags: []string{"boxy fit"}, Stock: 18, StockBySize: map[string]int{"P": 6, "M": 6, "G": 6}},
	{ID: "p-bb-look-black", Name: "BABY LOOK NAST TEE", Description: "Baby look preta em algodão 30.1 penteado com print cursivo \"Just be You\" frontal em branco. Corte ajustado feminino, gola reforçada e etiqueta tecida NAST.", PriceCents: 7990, PixPriceCents: 7591, Category: "Baby Look", Image: "bb-look-black.jpeg", BackImage: "bb-look-black-back.jpeg", Colors: []string{"preto"}, Sizes: []string{"Baby Look"}, Tags: []string{"edição limitada"}, Stock: 12, StockBySize: map[string]int{"Baby Look": 12}},
	{ID: "p-bb-look-white", Name: "BABY LOOK NAST TEE", Description: "Baby look branca em algodão 30.1 penteado com print cursivo \"Just be You\" frontal em preto. Corte ajustado feminino, gola reforçada e etiqueta tecida NAST.", PriceCents: 7990, PixPriceCents: 7591, Category: "Baby Look", Image: "bb-look-white.jpeg", BackImage: "bb-look-white-back.jpeg", Colors: []string{"branco"}, Sizes: []string{"Baby Look"}, Tags: []string{"edição limitada"}, Stock: 12, StockBySize: map[string]int{"Baby Look": 12}},
}

// ----- Rate limiter (token bucket per IP) -----

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	refill   float64 // tokens per second
}

func newRateLimiter(capacity, refillPerSecond float64) *rateLimiter {
	rl := &rateLimiter{
		buckets:  make(map[string]*bucket),
		capacity: capacity,
		refill:   refillPerSecond,
	}
	go rl.gc()
	return rl
}

func (rl *rateLimiter) gc() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, b := range rl.buckets {
			if now.Sub(b.lastRefill) > 15*time.Minute {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{tokens: rl.capacity, lastRefill: now}
		rl.buckets[ip] = b
	}
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = minFloat(rl.capacity, b.tokens+elapsed*rl.refill)
	b.lastRefill = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// ----- Middleware -----

// parseTrustedProxies parses a comma-separated list of CIDRs or single IPs.
// Invalid entries are silently dropped (they are logged at startup). Returns
// nil when the list is empty, which means "never trust X-Forwarded-For".
func parseTrustedProxies(raw string) []*net.IPNet {
	if raw == "" {
		return nil
	}
	var out []*net.IPNet
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if !strings.Contains(tok, "/") {
			if ip := net.ParseIP(tok); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				tok = fmt.Sprintf("%s/%d", tok, bits)
			}
		}
		_, netw, err := net.ParseCIDR(tok)
		if err != nil {
			log.Printf("trusted-proxies: ignoring invalid entry %q: %v", tok, err)
			continue
		}
		out = append(out, netw)
	}
	return out
}

// ipInTrusted reports whether ip is contained in any of the trusted ranges.
func ipInTrusted(ip net.IP, trusted []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP returns the best-effort originating IP for r. X-Forwarded-For is
// only honored when the immediate peer (r.RemoteAddr) is in one of the
// trustedProxies ranges. Because nginx/ALB/etc. append the real client IP
// to any pre-existing X-Forwarded-For, an attacker could spoof the left
// side of the chain; we walk the chain right-to-left and return the first
// IP that does NOT belong to a trusted range.
func clientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	peerHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peerHost = r.RemoteAddr
	}
	peerIP := net.ParseIP(peerHost)
	if !ipInTrusted(peerIP, trustedProxies) {
		return peerHost
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peerHost
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		ip := net.ParseIP(candidate)
		if ip == nil {
			// Header was tampered with; fall back to the peer.
			return peerHost
		}
		if !ipInTrusted(ip, trustedProxies) {
			return candidate
		}
	}
	// Entire chain was trusted infrastructure — bill the peer.
	return peerHost
}

// buildCSP returns the Content-Security-Policy value that matches the
// current deployment. When STATIC_DIR is set we serve a rendered React
// SPA that embeds Stripe Elements (js.stripe.com, hooks.stripe.com) and
// optionally Plausible analytics. The policy is still restrictive: no
// eval, no inline scripts, no arbitrary frames, image origins limited
// to self/data/https.
func buildCSP(servesSPA bool, plausibleSrc string) string {
	if !servesSPA {
		// API-only deploy: strictest policy. The backend returns JSON
		// and never renders HTML, so nothing legitimate needs any
		// resources.
		return "default-src 'none'; frame-ancestors 'none'"
	}
	scriptExtras := "https://js.stripe.com"
	connectExtras := "https://api.stripe.com"
	if plausibleSrc != "" {
		scriptExtras += " " + plausibleSrc
		// Plausible POSTs pageview events to the same origin as the
		// script by default.
		if u := plausibleOrigin(plausibleSrc); u != "" {
			connectExtras += " " + u
		}
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' " + scriptExtras,
		// Tailwind + Stripe Elements inject runtime <style> tags; we
		// allow 'unsafe-inline' for styles only — never for scripts.
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
		"font-src 'self' https://fonts.gstatic.com",
		"img-src 'self' data: https:",
		"connect-src 'self' " + connectExtras,
		"frame-src https://js.stripe.com https://hooks.stripe.com",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")
}

// plausibleOrigin extracts the scheme://host portion of a Plausible
// script URL so we can add it to connect-src.
func plausibleOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Accept full URLs only; silently ignore relative/garbled values.
	for _, scheme := range []string{"https://", "http://"} {
		if !strings.HasPrefix(raw, scheme) {
			continue
		}
		rest := raw[len(scheme):]
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			rest = rest[:i]
		}
		if rest == "" {
			return ""
		}
		return scheme + rest
	}
	return ""
}

func withSecurityHeaders(csp string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// withRecovery catches panics in downstream handlers, logs them with
// the request path and writes a sanitized 500 JSON response. The stack
// trace never reaches the client; it is only emitted to stderr for
// operators to triage.
func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: method=%s path=%s err=%v", r.Method, r.URL.Path, rec)
				// Don't try to write if headers already sent.
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal error"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// originMatcher matches an incoming Origin header against the
// ALLOWED_ORIGINS list. We support two patterns:
//
//   - Exact match: "https://nast.com.br"
//   - Leading wildcard: "https://*.vercel.app" matches any single-level
//     subdomain so Vercel preview deployments work without edits.
//
// Wildcards further up the hierarchy are intentionally not supported:
// a rogue "https://nast-phishing.vercel.app" preview would be accepted
// by the *.vercel.app entry and that's an accepted tradeoff for dev
// ergonomics, but we don't want it silently expanding to arbitrary
// subdomains of any ancestor.
type originMatcher struct {
	exact  string
	suffix string // set when pattern begins with "https://*."
}

func (m originMatcher) match(origin string) bool {
	if m.exact != "" {
		return origin == m.exact
	}
	if m.suffix == "" {
		return false
	}
	if !strings.HasPrefix(origin, "https://") && !strings.HasPrefix(origin, "http://") {
		return false
	}
	return strings.HasSuffix(origin, m.suffix)
}

func parseOrigins(raw string) []originMatcher {
	out := make([]originMatcher, 0)
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		// "https://*.vercel.app" → suffix ".vercel.app" (leading dot
		// kept to avoid matching "foovercel.app").
		if idx := strings.Index(o, "://*."); idx > 0 {
			suffix := o[idx+len("://*"):]
			out = append(out, originMatcher{suffix: suffix})
			continue
		}
		out = append(out, originMatcher{exact: o})
	}
	return out
}

func originAllowed(matchers []originMatcher, origin string) bool {
	for _, m := range matchers {
		if m.match(origin) {
			return true
		}
	}
	return false
}

func withCORS(allowed []originMatcher, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Vary: Origin is always emitted so intermediary caches key the
		// response on Origin even when this particular request was
		// same-origin / had no Origin header.
		w.Header().Set("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(allowed, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
			// Session cookies on /api/auth/* are sent via fetch() from
			// the SPA origin. Browsers only forward cookies when the
			// server opts in with ACAC=true *and* ACAO echoes the
			// actual origin (not "*"), so both conditions must hold.
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withRateLimit(rl *rateLimiter, trustedProxies []*net.IPNet, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r, trustedProxies)) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withBodyLimit(limit int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the outbound status
// code and response byte count for structured access logs.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// withLogging emits one structured JSON line per request on stdout.
// Fields are stable and cheap to parse from log aggregators (Fly logs,
// Loki, Datadog). We deliberately omit query strings and bodies to keep
// PII out of the stream.
func withLogging(trustedProxies []*net.IPNet, next http.Handler) http.Handler {
	enc := json.NewEncoder(os.Stdout)
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		entry := map[string]any{
			"t":        start.UTC().Format(time.RFC3339Nano),
			"level":    "info",
			"msg":      "http",
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   rec.status,
			"bytes":    rec.bytes,
			"duration": time.Since(start).Milliseconds(),
			"ip":       clientIP(r, trustedProxies),
		}
		if ref := r.Header.Get("Referer"); ref != "" {
			entry["referer"] = ref
		}
		mu.Lock()
		_ = enc.Encode(entry)
		mu.Unlock()
	})
}

// ----- Helpers -----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// staticOrNotFound returns the root handler. When STATIC_DIR points to a
// built SPA, file requests are served from disk and any path that doesn't
// match a file falls back to index.html so client-side routing works
// (/pedido/:token, /privacidade, /termos, …). When STATIC_DIR is empty
// — typical for local dev where the SPA is served by Vite on 5173 — the
// root just returns a JSON 404 so accidental hits don't leak anything.
func staticOrNotFound(dir string) http.HandlerFunc {
	if dir == "" {
		return func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	}
	fs := http.FileServer(http.Dir(dir))
	indexPath := dir + "/index.html"
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		// If the requested file exists under STATIC_DIR, serve it.
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean != "" {
			if fi, err := os.Stat(dir + "/" + clean); err == nil && !fi.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback.
		http.ServeFile(w, r, indexPath)
	}
}

func randomID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return prefix + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(buf)
}

func validateEmail(s string) bool {
	if len(s) < 3 || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	return strings.Contains(s[at:], ".")
}

func safeString(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || len(s) > max {
		return "", false
	}
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' {
			return "", false
		}
	}
	return s, true
}

// validateFullName rejects single-word names. SuperFrete refuses cart
// payloads where to.name is a single token (e.g. "João"), so we enforce
// the rule up front instead of discovering it at label-generation time.
// Accents, apostrophes and hyphens are allowed; extra whitespace between
// words is collapsed.
func validateFullName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || len(s) > 120 {
		return "", false
	}
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return "", false
	}
	for _, f := range fields {
		if len(f) < 2 {
			return "", false
		}
	}
	return strings.Join(fields, " "), true
}

// validateCPForCNPJ accepts a CPF (11 digits) or CNPJ (14 digits) and
// returns the digits-only form. Length is the only structural check —
// the upstream SuperFrete / Stripe stacks will reject checksum-invalid
// numbers, and rejecting here risks locking out legitimate numbers
// whose checksum library disagrees. Keeps the door open for CNPJ-based
// orders (small businesses) alongside the normal CPF customers.
func validateCPForCNPJ(s string) (string, bool) {
	d := digitsOnly(s)
	if len(d) != 11 && len(d) != 14 {
		return "", false
	}
	// Reject the trivial repeated-digit strings (e.g. 00000000000) that
	// are syntactically valid but never belong to a real document.
	allSame := true
	for i := 1; i < len(d); i++ {
		if d[i] != d[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return "", false
	}
	return d, true
}

// validateUF checks that s is a recognised 2-letter Brazilian state
// abbreviation (case-insensitive). The SuperFrete API rejects payloads
// where to.state_abbr is anything else, and the checkout form's
// autocomplete should only ever send valid UFs — but defense in depth.
func validateUF(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 2 {
		return "", false
	}
	switch s {
	case "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA",
		"MT", "MS", "MG", "PA", "PB", "PR", "PE", "PI", "RJ", "RN",
		"RS", "RO", "RR", "SC", "SP", "SE", "TO":
		return s, true
	}
	return "", false
}

// ----- Handlers -----

func handleProducts(store *productsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		list, err := store.listPublic(ctx, strings.TrimSpace(r.URL.Query().Get("category")))
		if err != nil {
			log.Printf("products list: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "catalog unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

func handleProductByID(store *productsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/products/")
		if id == "" || strings.Contains(id, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		p, err := store.get(ctx, id)
		if errors.Is(err, errProductNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "product not found"})
			return
		}
		if err != nil {
			log.Printf("product get: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func handleCheckout(orders *orderStore, products *productsStore, coupons *couponsStore, tokenKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req CheckoutRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if len(req.Items) == 0 || len(req.Items) > 50 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid item count"})
			return
		}
		name, ok := validateFullName(req.Name)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nome completo obrigatório"})
			return
		}
		email := strings.TrimSpace(req.Email)
		if !validateEmail(email) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email"})
			return
		}
		document, ok := validateCPForCNPJ(req.Document)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CPF/CNPJ inválido"})
			return
		}
		address, ok := safeString(req.Address, 240)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid address"})
			return
		}
		addressNumber, ok := safeString(req.AddressNumber, 24)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "número do endereço obrigatório"})
			return
		}
		// Complement is optional; empty is fine, but if present it has to
		// pass the safe-string check to keep control characters out.
		addressComplement := strings.TrimSpace(req.AddressComplement)
		if addressComplement != "" {
			var okC bool
			addressComplement, okC = safeString(addressComplement, 120)
			if !okC {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "complemento inválido"})
				return
			}
		}
		district, ok := safeString(req.District, 120)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bairro obrigatório"})
			return
		}
		city, ok := safeString(req.City, 120)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cidade obrigatória"})
			return
		}
		state, ok := validateUF(req.State)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "UF inválida"})
			return
		}
		zip, ok := safeString(req.ZipCode, 16)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid zip"})
			return
		}
		if len(digitsOnly(zip)) != 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CEP inválido"})
			return
		}
		payment := strings.TrimSpace(strings.ToLower(req.PaymentMethod))
		if payment == "" {
			payment = "card"
		}
		if payment != "pix" && payment != "card" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payment method"})
			return
		}
		var items []orderItem
		total := 0
		for _, it := range req.Items {
			if it.Quantity <= 0 || it.Quantity > 20 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid quantity"})
				return
			}
			ctxP, cancelP := context.WithTimeout(r.Context(), 2*time.Second)
			p, err := products.get(ctxP, it.ProductID)
			cancelP()
			if errors.Is(err, errProductNotFound) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown product"})
				return
			}
			if err != nil {
				log.Printf("checkout: product lookup: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "catalog unavailable"})
				return
			}
			// Validate that the requested size is actually offered by
			// the product and that enough inventory is available.
			// Products migrated with per-size stock enforce the map;
			// legacy rows (empty StockBySize) fall back to the total.
			if it.Size != "" && len(p.Sizes) > 0 {
				sizeOffered := false
				for _, s := range p.Sizes {
					if s == it.Size {
						sizeOffered = true
						break
					}
				}
				if !sizeOffered {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tamanho indisponível"})
					return
				}
			}
			if len(p.StockBySize) > 0 {
				if p.StockBySize[it.Size] < it.Quantity {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tamanho esgotado"})
					return
				}
			} else if p.Stock < it.Quantity {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "estoque insuficiente"})
				return
			}
			unit := p.PriceCents
			if payment == "pix" {
				unit = p.PixPriceCents
			}
			total += unit * it.Quantity
			items = append(items, orderItem{
				ProductID:      it.ProductID,
				ProductName:    p.Name,
				Size:           it.Size,
				Color:          it.Color,
				Quantity:       it.Quantity,
				UnitPriceCents: unit,
			})
		}
		shippingCents := 0
		var shipServiceID int
		var shipServiceName string
		if req.Shipping != nil {
			if req.Shipping.PriceCents < 0 || req.Shipping.PriceCents > 500_000 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid shipping price"})
				return
			}
			if n, okSN := safeString(req.Shipping.ServiceName, 80); okSN {
				shipServiceName = n
			}
			shippingCents = req.Shipping.PriceCents
			shipServiceID = req.Shipping.ServiceID
		}
		// Coupon (optional). Re-validate server-side even when the cart
		// already previewed the discount via /api/coupons/validate — we
		// trust nothing the browser sent.
		finalSubtotal := total
		finalShipping := shippingCents
		appliedCode := ""
		discountCents := 0
		if strings.TrimSpace(req.CouponCode) != "" {
			code := normalizeCouponCode(req.CouponCode)
			if code == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cupom inválido"})
				return
			}
			ctxC, cancelC := context.WithTimeout(r.Context(), 2*time.Second)
			c, errC := coupons.get(ctxC, code)
			cancelC()
			if errors.Is(errC, errCouponNotFound) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cupom não encontrado"})
				return
			}
			if errC != nil {
				log.Printf("checkout: coupon lookup: %v", errC)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "coupon unavailable"})
				return
			}
			if errE := couponEligibility(c, time.Now().UTC()); errE != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": errE.Error()})
				return
			}
			summary, errA := applyCoupon(c, total, shippingCents)
			if errA != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": errA.Error()})
				return
			}
			finalSubtotal = summary.NewSubtotalCents
			finalShipping = summary.NewShippingCents
			appliedCode = c.Code
			discountCents = summary.TotalDiscountCents
		}
		amount := finalSubtotal + finalShipping
		// Link the order to the authenticated customer when the
		// request arrived with a valid session cookie. Anonymous
		// checkouts continue to work (guest flow / auth disabled)
		// and keep UserID empty — those orders are still reachable
		// via /api/account/orders by email match.
		var userID string
		if u := currentUser(r); u != nil {
			userID = u.ID
		}
		order := &pendingOrder{
			ID:                randomID("ord_"),
			Name:              name,
			Email:             email,
			Document:          document,
			Address:           address,
			AddressNumber:     addressNumber,
			AddressComplement: addressComplement,
			District:          district,
			City:              city,
			State:             state,
			Zip:               zip,
			TotalCents:        finalSubtotal,
			ShippingCents:     finalShipping,
			AmountCents:       amount,
			ShippingSvcID:     shipServiceID,
			ShippingSvcName:   shipServiceName,
			PaymentMethod:     payment,
			Status:            "pending_payment",
			CouponCode:        appliedCode,
			DiscountCents:     discountCents,
			UserID:            userID,
			CreatedAt:         time.Now().UTC(),
			Items:             items,
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orders.create(ctx, order); err != nil {
			log.Printf("checkout: create order failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save order"})
			return
		}
		if appliedCode != "" {
			// Best-effort bump. A race here at the last use slot is
			// acceptable — max_uses is a soft budget and the admin can
			// deactivate the coupon if overshot.
			incCtx, cancelInc := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = coupons.db.ExecContext(incCtx,
				rb(`UPDATE coupons SET used_count = used_count + 1, updated_at = ? WHERE code = ?`),
				time.Now().UTC(), appliedCode)
			cancelInc()
		}
		token := ""
		if len(tokenKey) > 0 {
			token = makeOrderToken(tokenKey, order.ID, time.Now())
		}
		log.Printf("checkout: order=%s items=%d total=%d shipping=%d payment=%s zip=%s coupon=%s discount=%d",
			order.ID, len(order.Items), finalSubtotal, finalShipping, payment, zip, appliedCode, discountCents)
		writeJSON(w, http.StatusOK, CheckoutResponse{
			OrderID:       order.ID,
			OrderToken:    token,
			TotalCents:    finalSubtotal,
			ShippingCents: finalShipping,
			AmountCents:   amount,
			DiscountCents: discountCents,
			CouponCode:    appliedCode,
			Status:        order.Status,
			CreatedAt:     order.CreatedAt.Format(time.RFC3339),
			PaymentMethod: payment,
		})
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
}

// ----- Server bootstrap -----

// mapNonEmpty returns a if s is non-empty, b otherwise. Used for terse
// startup logging.
func mapNonEmpty(s, a, b string) string {
	if strings.TrimSpace(s) != "" {
		return a
	}
	return b
}

// orderTokenSecret returns the key used to HMAC order-lookup tokens. If
// ORDER_TOKEN_SECRET is set we use it verbatim. Otherwise we derive a
// key from STRIPE_SECRET_KEY (SHA-256 with a domain separator) so
// existing deployments get tokens for free without a new env var.
// Callers treat an empty result as "tokens disabled".
func orderTokenSecret() []byte {
	if v := strings.TrimSpace(os.Getenv("ORDER_TOKEN_SECRET")); v != "" {
		return []byte(v)
	}
	stripeKey := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if stripeKey == "" {
		return nil
	}
	h := sha256.New()
	h.Write([]byte("nast:order-tokens:v1"))
	h.Write([]byte(stripeKey))
	return h.Sum(nil)
}

// dbSentinel keeps the database/sql import genuinely used even in builds
// that happen to only reference openDB via the main wire-up. (Placeholder
// to appease unused-imports tooling during iterative edits.)
var _ *sql.DB

// ctxSentinel similar for context — referenced inside handlers via
// http.Request.Context but keeps the import grounded here.
var _ = context.Background

func allowedOriginsFromEnv() []originMatcher {
	raw := os.Getenv("ALLOWED_ORIGINS")
	if raw == "" {
		raw = "http://localhost:5173,http://127.0.0.1:5173"
	}
	return parseOrigins(raw)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	rl := newRateLimiter(60, 1) // 60-token bucket, refill 1 token/sec
	trustedProxies := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if len(trustedProxies) == 0 {
		log.Printf("TRUSTED_PROXIES not set — X-Forwarded-For ignored for rate limiting (rate limit keyed on direct peer)")
	} else {
		log.Printf("TRUSTED_PROXIES: %d range(s) configured", len(trustedProxies))
	}

	shipCfg := loadShippingConfig()
	if shipCfg.AccessToken == "" {
		log.Printf("SuperFrete: token not configured — /api/shipping/* will return 503")
	} else {
		log.Printf("SuperFrete: %s origin=%s", shipCfg.BaseURL, shipCfg.OriginZip)
	}
	shipClient := newShippingClient(shipCfg)
	shipCache := newQuoteCache(5 * time.Minute)

	payCfg := loadPaymentsConfig()
	if payCfg.SecretKey == "" {
		log.Printf("Stripe: secret key not configured — /api/payments/* will return 503")
	} else {
		log.Printf("Stripe: secret key loaded (webhook secret %s)",
			mapNonEmpty(payCfg.WebhookSecret, "configured", "missing — /api/payments/webhook will reject all events"))
	}
	stripeCli := newStripeClient(payCfg)

	db, err := openDB()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()
	orders := newOrderStore(db)
	products := newProductsStore(db)
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := seedProducts(seedCtx, products, catalog); err != nil {
		log.Printf("product seed: %v", err)
	}
	seedCancel()
	adminCfg := loadAdminAuthCfg()
	adminToken := adminCfg.legacyToken
	_ = adminToken // kept for logging/diagnostics if ever needed
	if adminCfg.legacyToken == "" && !adminCfg.loginEnabled() {
		log.Printf("admin: neither ADMIN_TOKEN nor ADMIN_USERNAME+ADMIN_PASSWORD_HASH set — /api/admin/* disabled")
	} else if adminCfg.loginEnabled() {
		log.Printf("admin: login enabled for user %q (sessions expire every %s)", adminCfg.username, adminCfg.sessionTTL)
	}

	tokenKey := orderTokenSecret()
	if len(tokenKey) == 0 {
		log.Printf("ORDER_TOKEN_SECRET not set — /api/orders/:token disabled and checkout responses omit orderToken")
	}

	appURL := strings.TrimSpace(os.Getenv("APP_URL"))
	if appURL == "" {
		appURL = "http://localhost:5173"
	}

	labelDisp := newSyncLabelDispatcher(orders, shipClient, labelTimeoutFromEnv())
	emailCfg := loadEmailConfig(tokenKey, appURL)
	emailWrk := newEmailWorker(emailCfg, orders)
	if emailCfg.APIKey == "" {
		log.Printf("Resend: RESEND_API_KEY not set — confirmation emails disabled")
	}
	// Fire the "order shipped + tracking" email on every successful
	// runLabelJob, regardless of which path triggered it (webhook
	// dispatcher, admin retry, or cron). Best-effort: any failure is
	// logged by the worker and never bubbled up to the caller.
	labelSuccessHook = emailWrk.EnqueueTracking

	internalCfg := loadInternalJobsConfig()
	if internalCfg.token == "" {
		log.Printf("internal jobs: INTERNAL_JOB_TOKEN / CRON_SECRET not set — /api/internal/jobs/* disabled")
	}

	webhookDeps := webhookDeps{
		Orders:   orders,
		Products: products,
		LabelJob: labelDisp,
		EmailJob: emailWrk,
		AppURL:   appURL,
		TokenKey: tokenKey,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/products", handleProducts(products))
	mux.HandleFunc("/api/products/", handleProductByID(products))
	coupons := newCouponsStore(db)
	mux.HandleFunc("/api/checkout", handleCheckout(orders, products, coupons, tokenKey))
	mux.HandleFunc("/api/coupons/validate", handleCouponValidate(coupons))
	mux.HandleFunc("/api/orders/", handleOrderLookup(orders, tokenKey))
	mux.HandleFunc("/api/payments/intent", handlePaymentsIntent(stripeCli, orders))
	mux.HandleFunc("/api/payments/webhook", handlePaymentsWebhook(payCfg, webhookDeps))
	mux.HandleFunc("/api/shipping/quote", handleShippingQuote(shipClient, shipCache))
	mux.HandleFunc("/api/shipping/label", handleShippingLabel(shipClient))
	mux.HandleFunc("/api/shipping/track/", handleShippingTrack(shipClient))
	mux.HandleFunc("/api/cep/", handleCepLookup(newViaCepClient()))
	igClient := newInstagramClient()
	if !igClient.configured() {
		log.Printf("instagram: INSTAGRAM_ACCESS_TOKEN + INSTAGRAM_USER_ID not set — feed widget will be hidden")
	}
	mux.HandleFunc("/api/social/instagram", handleInstagramFeed(igClient))
	settings := newSettingsStore(db)
	mux.HandleFunc("/api/settings", handlePublicSettings(settings))
	mux.HandleFunc("/api/admin/settings", adminAuthFromCfg(adminCfg, handleAdminSettings(settings)))
	mux.HandleFunc("/api/admin/login", handleAdminLogin(adminCfg))
	mux.HandleFunc("/api/admin/products", adminAuthFromCfg(adminCfg, handleAdminProducts(products)))
	mux.HandleFunc("/api/admin/products/", adminAuthFromCfg(adminCfg, handleAdminProductByID(products)))
	mux.HandleFunc("/api/admin/coupons", adminAuthFromCfg(adminCfg, handleAdminCoupons(coupons)))
	mux.HandleFunc("/api/admin/coupons/", adminAuthFromCfg(adminCfg, handleAdminCouponByCode(coupons)))
	mux.HandleFunc("/api/admin/stats", adminAuthFromCfg(adminCfg, handleAdminStats(db)))
	mux.HandleFunc("/api/admin/upload", adminAuthFromCfg(adminCfg, handleAdminUpload()))
	mux.HandleFunc("/api/admin/orders", adminAuthFromCfg(adminCfg, handleAdminOrdersList(orders)))
	mux.HandleFunc("/api/admin/orders/pending-labels", adminAuthFromCfg(adminCfg, handleAdminPendingLabels(orders)))
	viacepClient := newViaCepClient()
	mux.HandleFunc("/api/admin/orders/", adminAuthFromCfg(adminCfg, handleAdminOrderActions(orders, shipClient, viacepClient, labelTimeoutFromEnv())))
	mux.HandleFunc("/api/internal/jobs/process-labels", handleProcessLabelsJob(internalCfg, orders, shipClient, viacepClient))

	// Customer auth (email/password + Google OAuth). 503s until
	// AUTH_SESSION_SECRET is configured so the surface stays inert
	// in deployments that haven't opted in yet.
	authCfg := loadAuthConfig()
	gcfg := loadGoogleConfig()
	users := newUserStore(db)
	if !authCfg.enabled() {
		log.Printf("auth: AUTH_SESSION_SECRET not set — /api/auth/* returns 503")
	} else if !gcfg.enabled() {
		log.Printf("auth: GOOGLE_CLIENT_ID/SECRET/REDIRECT_URL not set — Google sign-in disabled (email/password still works)")
	}
	mux.HandleFunc("/api/auth/signup", handleSignup(authCfg, users, orders))
	mux.HandleFunc("/api/auth/login", handleLogin(authCfg, users, orders))
	mux.HandleFunc("/api/auth/logout", handleLogout(authCfg, users))
	mux.HandleFunc("/api/auth/me", handleMe)
	mux.HandleFunc("/api/auth/google/start", handleGoogleStart(authCfg, gcfg))
	mux.HandleFunc("/api/auth/google/callback", handleGoogleCallback(authCfg, gcfg, users, orders))
	// Account endpoints — behind requireAuth, shared 503 short-circuit
	// when auth is not configured so the /minha-conta page falls back
	// to the friendly "login indisponível" message instead of an
	// opaque 401.
	mux.HandleFunc("/api/account/orders", handleMyOrders(authCfg, orders, tokenKey))

	staticDir := strings.TrimSpace(os.Getenv("STATIC_DIR"))
	mux.HandleFunc("/", staticOrNotFound(staticDir))

	plausibleSrc := strings.TrimSpace(os.Getenv("PLAUSIBLE_SCRIPT_SRC"))
	csp := buildCSP(staticDir != "", plausibleSrc)

	var h http.Handler = mux
	// loadCurrentUser attaches the authenticated userAccount to the
	// request context when a valid session cookie is present. It is
	// a no-op when auth is disabled or the user is anonymous.
	h = loadCurrentUser(authCfg, users)(h)
	h = withBodyLimit(1<<16, h) // 64KiB
	h = withRateLimit(rl, trustedProxies, h)
	h = withCORS(allowedOriginsFromEnv(), h)
	h = withSecurityHeaders(csp, h)
	h = withRecovery(h)
	h = withLogging(trustedProxies, h)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 14, // 16KiB
	}

	log.Printf("NAST backend listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
