package main

// Admin endpoints for triaging orders that didn't make it to "shipped".
//
// /api/admin/orders/pending-labels  GET  list paid orders without tracking
// /api/admin/orders/{id}/retry-label POST run runLabelJob synchronously
//
// All routes are gated by the existing adminAuth middleware (legacy
// X-Admin-Token or session cookie). The pending-labels list deliberately
// omits items that are already "shipped" or still "pending_payment" —
// the operator only cares about what got stuck after a successful
// payment.

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// adminPendingLabel is the JSON shape returned by the listing
// endpoint. Mirrors pendingOrder fields the operator needs to
// triage — we don't dump the whole row to avoid leaking address PII
// into logs/screenshots that may travel.
type adminPendingLabel struct {
	OrderID             string    `json:"orderId"`
	Name                string    `json:"name"`
	Email               string    `json:"email"`
	Status              string    `json:"status"`
	PaymentMethod       string    `json:"paymentMethod"`
	AmountCents         int       `json:"amountCents"`
	ShippingServiceID   int       `json:"shippingServiceId"`
	ShippingServiceName string    `json:"shippingServiceName"`
	TrackingAttempts    int       `json:"trackingAttempts"`
	TrackingLastError   string    `json:"trackingLastError,omitempty"`
	TrackingAttemptedAt time.Time `json:"trackingAttemptedAt,omitempty"`
	CreatedAt           time.Time `json:"createdAt"`
}

func handleAdminPendingLabels(orders *orderStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		// Generous cutoff (now) so the admin sees every paid-without-
		// tracking row regardless of cooldown — the cron handles the
		// actual backoff. We also lift the maxAttempts ceiling so
		// permanently-stuck orders still surface for manual fix-up.
		rows, err := orders.listPendingLabels(ctx, time.Now().UTC(), 1<<30, 200)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
			return
		}
		out := make([]adminPendingLabel, 0, len(rows))
		for _, o := range rows {
			out = append(out, adminPendingLabel{
				OrderID:             o.ID,
				Name:                o.Name,
				Email:               o.Email,
				Status:              o.Status,
				PaymentMethod:       o.PaymentMethod,
				AmountCents:         o.AmountCents,
				ShippingServiceID:   o.ShippingSvcID,
				ShippingServiceName: o.ShippingSvcName,
				TrackingAttempts:    o.TrackingAttempts,
				TrackingLastError:   o.TrackingLastError,
				TrackingAttemptedAt: o.TrackingAttemptedAt,
				CreatedAt:           o.CreatedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"count":  len(out),
			"orders": out,
		})
	}
}

// handleAdminOrderActions dispatches sub-routes under
// /api/admin/orders/{id}/... . Today only the retry-label sub-route is
// implemented; future moves (manual cancel, edit address) plug in here.
func handleAdminOrderActions(orders *orderStore, ship *shippingClient, perOrder time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/admin/orders/")
		// The pending-labels listing is registered with an exact
		// pattern that wins over this prefix in net/http.ServeMux,
		// but defensively skip it here too.
		if path == "" || path == "pending-labels" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		parts := strings.Split(path, "/")
		if len(parts) != 2 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		orderID, action := parts[0], parts[1]
		if orderID == "" || strings.ContainsAny(orderID, "?#") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order id"})
			return
		}
		switch action {
		case "retry-label":
			adminRetryLabel(w, r, orders, ship, orderID, perOrder)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
		}
	}
}

func adminRetryLabel(w http.ResponseWriter, r *http.Request, orders *orderStore, ship *shippingClient, orderID string, perOrder time.Duration) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if ship == nil || ship.cfg.AccessToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shipping unavailable"})
		return
	}
	if perOrder <= 0 {
		perOrder = 25 * time.Second
	}
	// Slightly more generous than the cron — operators clicking the
	// retry button want feedback that the call actually finished.
	ctx, cancel := context.WithTimeout(r.Context(), perOrder+10*time.Second)
	defer cancel()
	if err := runLabelJob(ctx, orders, ship, orderID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "retry failed",
			"orderId": orderID,
			"detail":  err.Error(),
		})
		return
	}
	o, ok, err := orders.get(ctx, orderID)
	if err != nil || !ok {
		writeJSON(w, http.StatusOK, map[string]any{"orderId": orderID, "status": "ok"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orderId":      o.ID,
		"status":       o.Status,
		"trackingCode": o.TrackingCode,
		"trackingUrl":  o.TrackingURL,
		"labelUrl":     o.LabelURL,
		"superfreteId": o.SuperfreteID,
	})
}
