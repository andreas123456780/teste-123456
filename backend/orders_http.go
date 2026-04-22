package main

// Customer-facing order lookup.
//
// The checkout flow mints an HMAC-signed token pointing at an order; the
// frontend renders /pedido/<token> and hits GET /api/orders/<token> from
// there. This endpoint returns only the fields the customer should see —
// never the full email, address, or PaymentIntent secret.

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// publicOrder is the sanitized shape we return to the frontend. Anything
// not listed here stays server-side.
type publicOrder struct {
	OrderID        string             `json:"orderId"`
	Status         string             `json:"status"`
	PaymentMethod  string             `json:"paymentMethod"`
	TotalCents     int                `json:"totalCents"`
	ShippingCents  int                `json:"shippingCents"`
	AmountCents    int                `json:"amountCents"`
	ShippingName   string             `json:"shippingName,omitempty"`
	TrackingCode   string             `json:"trackingCode,omitempty"`
	TrackingURL    string             `json:"trackingUrl,omitempty"`
	CreatedAt      string             `json:"createdAt"`
	UpdatedAt      string             `json:"updatedAt"`
	CustomerMasked string             `json:"customer"`
	Items          []publicOrderItem  `json:"items"`
}

type publicOrderItem struct {
	ProductID      string `json:"productId"`
	ProductName    string `json:"productName"`
	Size           string `json:"size,omitempty"`
	Color          string `json:"color,omitempty"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int    `json:"unitPriceCents"`
}

func maskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return ""
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 2 {
		return strings.Repeat("•", len(local)) + domain
	}
	return string(local[0]) + strings.Repeat("•", len(local)-2) + string(local[len(local)-1]) + domain
}

// handleOrderLookup serves GET /api/orders/:token.
func handleOrderLookup(orders *orderStore, tokenKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if len(tokenKey) == 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "order lookup unavailable"})
			return
		}
		token := strings.TrimPrefix(r.URL.Path, "/api/orders/")
		if token == "" || strings.ContainsAny(token, "/?#") || len(token) > 512 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token"})
			return
		}
		orderID, err := parseOrderToken(tokenKey, token, time.Now())
		if err != nil {
			// Do not leak which reason failed — the client only needs
			// to know the link is bad.
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		o, ok, err := orders.get(r.Context(), orderID)
		if err != nil {
			log.Printf("orders.lookup store error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
			return
		}
		out := publicOrder{
			OrderID:        o.ID,
			Status:         o.Status,
			PaymentMethod:  o.PaymentMethod,
			TotalCents:     o.TotalCents,
			ShippingCents:  o.ShippingCents,
			AmountCents:    o.AmountCents,
			ShippingName:   o.ShippingSvcName,
			TrackingCode:   o.TrackingCode,
			TrackingURL:    o.TrackingURL,
			CreatedAt:      o.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      o.UpdatedAt.UTC().Format(time.RFC3339),
			CustomerMasked: maskEmail(o.Email),
		}
		for _, it := range o.Items {
			out.Items = append(out.Items, publicOrderItem{
				ProductID:      it.ProductID,
				ProductName:    it.ProductName,
				Size:           it.Size,
				Color:          it.Color,
				Quantity:       it.Quantity,
				UnitPriceCents: it.UnitPriceCents,
			})
		}
		// Allow moderate caching on the customer's browser but nothing
		// shared — order records contain personal fields even masked.
		w.Header().Set("Cache-Control", "private, max-age=15")
		writeJSON(w, http.StatusOK, out)
	}
}
