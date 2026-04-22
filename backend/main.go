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
	"crypto/rand"
	"crypto/subtle"
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
}

type CartItem struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
	Size      string `json:"size"`
	Color     string `json:"color"`
}

type CheckoutRequest struct {
	Items         []CartItem `json:"items"`
	Name          string     `json:"name"`
	Email         string     `json:"email"`
	Address       string     `json:"address"`
	ZipCode       string     `json:"zipCode"`
	PaymentMethod string     `json:"paymentMethod"`
}

type CheckoutResponse struct {
	OrderID     string `json:"orderId"`
	TotalCents  int    `json:"totalCents"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	EstimatedAt string `json:"estimatedAt"`
}

// ----- In-memory catalog (seeded on startup) -----

// NAST streetwear · edição limitada — 4 peças.
var catalog = []Product{
	{ID: "p-tee-bw-black", Name: "CAMISETA BLACK & WHITE", Description: "Camiseta preta em algodão 30.1 penteado com print cursivo frontal em branco. Corte regular, gola reforçada.", PriceCents: 8990, PixPriceCents: 8541, Category: "Camisetas", Image: "tee-cursive-black.jpg", BackImage: "tee-cursive-black.jpg", Colors: []string{"preto"}, Sizes: []string{"P", "M", "G", "Baby Look"}, Tags: []string{"edição limitada"}, Stock: 24},
	{ID: "p-tee-bw-white", Name: "CAMISA BLACK & WHITE", Description: "Camiseta branca em algodão 30.1 penteado com print cursivo frontal em preto. Corte regular, gola reforçada.", PriceCents: 8990, PixPriceCents: 8541, Category: "Camisetas", Image: "tee-cursive-white.png", BackImage: "tee-cursive-white.png", Colors: []string{"branco"}, Sizes: []string{"P", "M", "G", "Baby Look"}, Tags: []string{"edição limitada"}, Stock: 24},
	{ID: "p-boxy-black", Name: "CAMISA BOXY NAST PRETA", Description: "Camiseta boxy preta em algodão pesado 240g com modelagem oversized, ombro caído e etiqueta tecida NAST.", PriceCents: 9990, PixPriceCents: 9491, Category: "Boxy", Image: "boxy-black.jpg", BackImage: "boxy-black.jpg", Colors: []string{"preto"}, Sizes: []string{"P", "M", "G"}, Tags: []string{"boxy fit"}, Stock: 18},
	{ID: "p-boxy-white", Name: "CAMISETA BOXY NAST BRANCA", Description: "Camiseta boxy branca em algodão pesado 240g com modelagem oversized, ombro caído e etiqueta tecida NAST.", PriceCents: 9990, PixPriceCents: 9491, Category: "Boxy", Image: "boxy-white.jpg", BackImage: "boxy-white.jpg", Colors: []string{"branco"}, Sizes: []string{"P", "M", "G"}, Tags: []string{"boxy fit"}, Stock: 18},
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

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func withCORS(allowed map[string]struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Vary: Origin is always emitted so intermediary caches key the
		// response on Origin even when this particular request was
		// same-origin / had no Origin header.
		w.Header().Set("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
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

func withLogging(trustedProxies []*net.IPNet, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %s", clientIP(r, trustedProxies), r.Method, r.URL.Path, time.Since(start))
	})
}

// ----- Helpers -----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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

// ----- Handlers -----

func handleProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	out := catalog
	if category != "" {
		out = make([]Product, 0, len(catalog))
		for _, p := range catalog {
			if strings.EqualFold(p.Category, category) {
				out = append(out, p)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func handleProductByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/products/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	for _, p := range catalog {
		// Constant-time compare to make ID probing uniform in timing.
		if subtle.ConstantTimeCompare([]byte(p.ID), []byte(id)) == 1 {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "product not found"})
}

func handleCheckout(w http.ResponseWriter, r *http.Request) {
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
	name, ok := safeString(req.Name, 120)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid name"})
		return
	}
	if !validateEmail(strings.TrimSpace(req.Email)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email"})
		return
	}
	address, ok := safeString(req.Address, 240)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid address"})
		return
	}
	zip, ok := safeString(req.ZipCode, 16)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid zip"})
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
	total := 0
	for _, it := range req.Items {
		if it.Quantity <= 0 || it.Quantity > 20 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid quantity"})
			return
		}
		found := false
		for _, p := range catalog {
			if p.ID == it.ProductID {
				unit := p.PriceCents
				if payment == "pix" {
					unit = p.PixPriceCents
				}
				total += unit * it.Quantity
				found = true
				break
			}
		}
		if !found {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown product"})
			return
		}
	}
	resp := CheckoutResponse{
		OrderID:     randomID("ord_"),
		TotalCents:  total,
		Status:      "confirmed",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		EstimatedAt: time.Now().Add(5 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}
	log.Printf("checkout: order=%s name=%q email=%q items=%d total=%d payment=%s zip=%s address-len=%d",
		resp.OrderID, name, req.Email, len(req.Items), total, payment, zip, len(address))
	writeJSON(w, http.StatusOK, resp)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
}

// ----- Server bootstrap -----

func allowedOriginsFromEnv() map[string]struct{} {
	raw := os.Getenv("ALLOWED_ORIGINS")
	if raw == "" {
		raw = "http://localhost:5173,http://127.0.0.1:5173"
	}
	out := make(map[string]struct{})
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			out[o] = struct{}{}
		}
	}
	return out
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
		log.Printf("Melhor Envio: token not configured — /api/shipping/* will return 503")
	} else {
		log.Printf("Melhor Envio: %s origin=%s", shipCfg.BaseURL, shipCfg.OriginZip)
	}
	shipClient := newShippingClient(shipCfg)
	shipCache := newQuoteCache(5 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/products", handleProducts)
	mux.HandleFunc("/api/products/", handleProductByID)
	mux.HandleFunc("/api/checkout", handleCheckout)
	mux.HandleFunc("/api/shipping/quote", handleShippingQuote(shipClient, shipCache))
	mux.HandleFunc("/api/shipping/label", handleShippingLabel(shipClient))
	mux.HandleFunc("/api/shipping/track/", handleShippingTrack(shipClient))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})

	var h http.Handler = mux
	h = withBodyLimit(1<<16, h) // 64KiB
	h = withRateLimit(rl, trustedProxies, h)
	h = withCORS(allowedOriginsFromEnv(), h)
	h = withSecurityHeaders(h)
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
