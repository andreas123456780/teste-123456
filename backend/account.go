package main

// Customer-facing account endpoints. Backs the /minha-conta page
// introduced in PR C. All endpoints here require an authenticated
// session (via requireAuth); we short-circuit with 503 when auth
// isn't configured so the frontend can show a friendly "login
// indisponível" message instead of swallowing a bare 401.

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// myOrderItem is the per-line-item shape returned by the account
// endpoint. Keep it narrow — the customer doesn't need SuperFrete
// internals; they get the tracking code + link + a product line
// summary for rendering.
type myOrderItem struct {
	ProductID      string `json:"productId"`
	ProductName    string `json:"productName"`
	Size           string `json:"size,omitempty"`
	Color          string `json:"color,omitempty"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int    `json:"unitPriceCents"`
}

// myOrder is the per-order shape returned by the account endpoint.
// Tokens (/pedido/:token) are regenerated fresh so each page load
// gives the customer a working link even if the original email's
// token has long since dropped out of any inbox search.
type myOrder struct {
	ID              string        `json:"id"`
	OrderToken      string        `json:"orderToken,omitempty"`
	Status          string        `json:"status"`
	PaymentMethod   string        `json:"paymentMethod"`
	TotalCents      int           `json:"totalCents"`
	ShippingCents   int           `json:"shippingCents"`
	DiscountCents   int           `json:"discountCents,omitempty"`
	AmountCents     int           `json:"amountCents"`
	CouponCode      string        `json:"couponCode,omitempty"`
	TrackingCode    string        `json:"trackingCode,omitempty"`
	TrackingURL     string        `json:"trackingUrl,omitempty"`
	ShippingService string        `json:"shippingService,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
	Items           []myOrderItem `json:"items"`
}

// handleMyOrders returns every order belonging to the current user.
// Combines two lookups: orders.user_id == u.ID (canonical) plus
// orders.email == u.emailLower (legacy, pre-PR-C purchases). The
// store's linkOrdersByEmail — called on signup/login — eventually
// folds the second case into the first, so the email fallback is
// mostly a safety net.
func handleMyOrders(cfg authConfig, orders *orderStore, tokenKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
			return
		}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		u := currentUser(r)
		if u == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		list, err := orders.listByUser(ctx, u.ID, strings.ToLower(u.Email))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		out := make([]myOrder, 0, len(list))
		now := time.Now()
		for _, o := range list {
			out = append(out, toMyOrder(o, tokenKey, now))
		}
		writeJSON(w, http.StatusOK, map[string]any{"orders": out})
	}
}

// toMyOrder projects a pendingOrder down to the customer-facing
// shape, generating a fresh order token so the customer can jump
// straight from /minha-conta to /pedido/:token.
func toMyOrder(o *pendingOrder, tokenKey []byte, now time.Time) myOrder {
	items := make([]myOrderItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, myOrderItem{
			ProductID:      it.ProductID,
			ProductName:    it.ProductName,
			Size:           it.Size,
			Color:          it.Color,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		})
	}
	tok := ""
	if len(tokenKey) > 0 {
		tok = makeOrderToken(tokenKey, o.ID, now)
	}
	return myOrder{
		ID:              o.ID,
		OrderToken:      tok,
		Status:          o.Status,
		PaymentMethod:   o.PaymentMethod,
		TotalCents:      o.TotalCents,
		ShippingCents:   o.ShippingCents,
		DiscountCents:   o.DiscountCents,
		AmountCents:     o.AmountCents,
		CouponCode:      o.CouponCode,
		TrackingCode:    o.TrackingCode,
		TrackingURL:     o.TrackingURL,
		ShippingService: o.ShippingSvcName,
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
		Items:           items,
	}
}
