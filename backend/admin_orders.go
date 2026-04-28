package main

// Admin endpoints for triaging orders that didn't make it to "shipped".
//
// /api/admin/orders                     GET  list all orders (paginated)
// /api/admin/orders/pending-labels      GET  list paid orders without tracking
// /api/admin/orders/{id}                GET  full order details (items + PII)
// /api/admin/orders/{id}                DELETE remove an order and its items
// /api/admin/orders/{id}/retry-label    POST run runLabelJob synchronously
// /api/admin/orders/{id}/refresh-tracking POST poll SuperFrete for tracking code
// /api/admin/orders/{id}/mark-shipped   POST save manual tracking + email buyer
//
// All routes are gated by the existing adminAuth middleware (legacy
// X-Admin-Token or session cookie). The pending-labels list deliberately
// omits items that are already "shipped" or still "pending_payment" —
// the operator only cares about what got stuck after a successful
// payment.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

// adminOrderDetail is the JSON shape returned by list/detail. Includes
// full recipient PII (CPF + address) + line items so the dashboard can
// print picking slips and populate SuperFrete retries without a second
// round-trip.
type adminOrderDetail struct {
	OrderID             string          `json:"orderId"`
	Status              string          `json:"status"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
	Name                string          `json:"name"`
	Email               string          `json:"email"`
	Document            string          `json:"document,omitempty"`
	Address             string          `json:"address"`
	AddressNumber       string          `json:"addressNumber,omitempty"`
	AddressComplement   string          `json:"addressComplement,omitempty"`
	District            string          `json:"district,omitempty"`
	City                string          `json:"city,omitempty"`
	State               string          `json:"state,omitempty"`
	Zip                 string          `json:"zip"`
	PaymentMethod       string          `json:"paymentMethod"`
	TotalCents          int             `json:"totalCents"`
	ShippingCents       int             `json:"shippingCents"`
	AmountCents         int             `json:"amountCents"`
	DiscountCents       int             `json:"discountCents,omitempty"`
	CouponCode          string          `json:"couponCode,omitempty"`
	ShippingServiceID   int             `json:"shippingServiceId,omitempty"`
	ShippingServiceName string          `json:"shippingServiceName,omitempty"`
	TrackingCode        string          `json:"trackingCode,omitempty"`
	TrackingURL         string          `json:"trackingUrl,omitempty"`
	LabelURL            string          `json:"labelUrl,omitempty"`
	SuperfreteID        string          `json:"superfreteId,omitempty"`
	TrackingAttempts    int             `json:"trackingAttempts,omitempty"`
	TrackingLastError   string          `json:"trackingLastError,omitempty"`
	Items               []adminOrderItem `json:"items"`
}

type adminOrderItem struct {
	ProductID      string `json:"productId"`
	ProductName    string `json:"productName"`
	Size           string `json:"size,omitempty"`
	Color          string `json:"color,omitempty"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int    `json:"unitPriceCents"`
}

func toAdminOrderDetail(o *pendingOrder) adminOrderDetail {
	items := make([]adminOrderItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, adminOrderItem{
			ProductID:      it.ProductID,
			ProductName:    it.ProductName,
			Size:           it.Size,
			Color:          it.Color,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		})
	}
	return adminOrderDetail{
		OrderID:             o.ID,
		Status:              o.Status,
		CreatedAt:           o.CreatedAt,
		UpdatedAt:           o.UpdatedAt,
		Name:                o.Name,
		Email:               o.Email,
		Document:            o.Document,
		Address:             o.Address,
		AddressNumber:       o.AddressNumber,
		AddressComplement:   o.AddressComplement,
		District:            o.District,
		City:                o.City,
		State:               o.State,
		Zip:                 o.Zip,
		PaymentMethod:       o.PaymentMethod,
		TotalCents:          o.TotalCents,
		ShippingCents:       o.ShippingCents,
		AmountCents:         o.AmountCents,
		DiscountCents:       o.DiscountCents,
		CouponCode:          o.CouponCode,
		ShippingServiceID:   o.ShippingSvcID,
		ShippingServiceName: o.ShippingSvcName,
		TrackingCode:        o.TrackingCode,
		TrackingURL:         o.TrackingURL,
		LabelURL:            o.LabelURL,
		SuperfreteID:        o.SuperfreteID,
		TrackingAttempts:    o.TrackingAttempts,
		TrackingLastError:   o.TrackingLastError,
		Items:               items,
	}
}

// handleAdminOrdersList powers the /api/admin/orders listing. Supports
// pagination (?limit, ?offset) and optional status filter (?status=paid
// or pending_payment / shipped / canceled). Returns the full recipient
// detail so the UI can render an orders table with CPF + address
// without a second round-trip per row.
func handleAdminOrdersList(orders *orderStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		limit := parseIntOrDefault(r.URL.Query().Get("limit"), 50)
		offset := parseIntOrDefault(r.URL.Query().Get("offset"), 0)
		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
		defer cancel()
		rows, err := orders.listOrders(ctx, status, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
			return
		}
		out := make([]adminOrderDetail, 0, len(rows))
		for _, o := range rows {
			out = append(out, toAdminOrderDetail(o))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"count":  len(out),
			"orders": out,
		})
	}
}

// parseIntOrDefault is a tiny helper to keep the admin handlers
// readable — strconv.Atoi + fallback is noisy inline.
func parseIntOrDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
		if n > 1<<20 {
			return def
		}
	}
	return n
}

// handleAdminOrderActions dispatches sub-routes under
// /api/admin/orders/{id}[/action]. The admin dashboard uses:
//   - GET    /api/admin/orders/{id}            → full detail
//   - DELETE /api/admin/orders/{id}            → remove (for cleanup)
//   - POST   /api/admin/orders/{id}/retry-label → reissue label
func handleAdminOrderActions(orders *orderStore, ship *shippingClient, viacep *viaCepClient, perOrder time.Duration) http.HandlerFunc {
	if viacep == nil {
		viacep = newViaCepClient()
	}
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
		if len(parts) == 0 || parts[0] == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order id"})
			return
		}
		orderID := parts[0]
		if strings.ContainsAny(orderID, "?#") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order id"})
			return
		}
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodGet:
				adminOrderDetailHandler(w, r, orders, orderID)
			case http.MethodDelete:
				adminOrderDeleteHandler(w, r, orders, orderID)
			default:
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if len(parts) != 2 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		switch parts[1] {
		case "retry-label":
			adminRetryLabel(w, r, orders, ship, viacep, orderID, perOrder)
		case "refresh-tracking":
			adminRefreshTracking(w, r, orders, ship, orderID)
		case "mark-shipped":
			adminMarkShipped(w, r, orders, orderID)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
		}
	}
}

func adminOrderDetailHandler(w http.ResponseWriter, r *http.Request, orders *orderStore, orderID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	o, ok, err := orders.get(ctx, orderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
		return
	}
	writeJSON(w, http.StatusOK, toAdminOrderDetail(o))
}

func adminOrderDeleteHandler(w http.ResponseWriter, r *http.Request, orders *orderStore, orderID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := orders.deleteOrder(ctx, orderID); err != nil {
		if errors.Is(err, errOrderNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminRetryLabelBody is the optional JSON payload the admin retry
// endpoint accepts. Any field left empty falls back to the row stored
// on the order (for district/city/state the backend additionally
// consults ViaCEP). Primarily used to fix up orders where the checkout
// form captured a first-name-only recipient, which SuperFrete rejects.
type adminRetryLabelBody struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	// Document is the recipient CPF (11 digits) or CNPJ (14 digits).
	// SuperFrete requires it on the majority of accounts. Any
	// non-digit characters are stripped before the call.
	Document string `json:"document"`
	// Split address details. Leave any of these blank to let ViaCEP
	// resolve from the stored zip.
	District string `json:"district"`
	City     string `json:"city"`
	State    string `json:"state"`
}

func adminRetryLabel(w http.ResponseWriter, r *http.Request, orders *orderStore, ship *shippingClient, viacep *viaCepClient, orderID string, perOrder time.Duration) {
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

	// Parse the optional overrides body. An empty body is fine — the
	// common case is "just retry it".
	overrides := labelOverrides{}
	if r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		_ = r.Body.Close()
		if len(bytes.TrimSpace(raw)) > 0 {
			var body adminRetryLabelBody
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			overrides.Name = strings.TrimSpace(body.Name)
			overrides.Address = strings.TrimSpace(body.Address)
			overrides.Document = strings.TrimSpace(body.Document)
			overrides.Details = addressDetails{
				District: strings.TrimSpace(body.District),
				City:     strings.TrimSpace(body.City),
				State:    strings.ToUpper(strings.TrimSpace(body.State)),
			}
		}
	}

	// Slightly more generous than the cron — operators clicking the
	// retry button want feedback that the call actually finished.
	ctx, cancel := context.WithTimeout(r.Context(), perOrder+10*time.Second)
	defer cancel()
	if err := runLabelJob(ctx, orders, ship, viacep, orderID, overrides); err != nil {
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

// adminRefreshTracking polls SuperFrete's /order/info endpoint for a
// tracking code. Used when the operator has just paid the label via
// the SuperFrete app and wants to finalize the order without retyping
// the tracking code. If the carrier has already issued the code we
// persist it, flip the order to "shipped", and fire the tracking
// email. Otherwise we return the current SuperFrete status so the UI
// can tell the operator "still waiting".
func adminRefreshTracking(w http.ResponseWriter, r *http.Request, orders *orderStore, ship *shippingClient, orderID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if ship == nil || ship.cfg.AccessToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shipping unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	o, ok, err := orders.get(ctx, orderID)
	if err != nil || !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
		return
	}
	if o.SuperfreteID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "order has no SuperFrete id — generate the label first",
		})
		return
	}
	if o.TrackingCode != "" {
		// Already finalized — return the current state so the UI can
		// reflect whatever is on disk.
		writeJSON(w, http.StatusOK, map[string]any{
			"orderId":      o.ID,
			"status":       o.Status,
			"trackingCode": o.TrackingCode,
			"trackingUrl":  o.TrackingURL,
			"updated":      false,
		})
		return
	}

	info, err := ship.OrderInfo(ctx, o.SuperfreteID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  "superfrete order/info failed",
			"detail": err.Error(),
		})
		return
	}
	code, url := parseTrackingFromInfo(info)
	if code == "" {
		// SuperFrete hasn't issued a tracking code yet — usually
		// means the label wasn't paid. Surface the raw response so
		// the frontend can show it in a debug collapse.
		writeJSON(w, http.StatusOK, map[string]any{
			"orderId":  o.ID,
			"status":   o.Status,
			"updated":  false,
			"rawInfo":  json.RawMessage(info),
			"hint":     "label ainda não foi paga no SuperFrete",
		})
		return
	}
	if err := orders.setTracking(ctx, orderID, code, url, o.LabelURL, o.SuperfreteID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "persist tracking failed"})
		return
	}
	if err := orders.setStatus(ctx, orderID, "shipped"); err != nil {
		// Non-fatal — tracking is already saved.
		writeJSON(w, http.StatusOK, map[string]any{
			"orderId":      o.ID,
			"status":       o.Status,
			"trackingCode": code,
			"trackingUrl":  url,
			"updated":      true,
			"warning":      "tracking saved but status flip failed",
		})
		return
	}
	if h := labelSuccessHook; h != nil {
		h(orderID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orderId":      o.ID,
		"status":       "shipped",
		"trackingCode": code,
		"trackingUrl":  url,
		"updated":      true,
	})
}

// adminMarkShippedBody is the payload for manual tracking entry. Used
// when the operator prefers to type the Correios code directly (e.g.
// the label was paid outside SuperFrete or /order/info hasn't
// propagated yet).
type adminMarkShippedBody struct {
	TrackingCode string `json:"trackingCode"`
	TrackingURL  string `json:"trackingUrl"`
}

func adminMarkShipped(w http.ResponseWriter, r *http.Request, orders *orderStore, orderID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	_ = r.Body.Close()
	var body adminMarkShippedBody
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
	}
	code := strings.TrimSpace(body.TrackingCode)
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "trackingCode required"})
		return
	}
	url := strings.TrimSpace(body.TrackingURL)
	if url == "" {
		url = "https://rastreamento.correios.com.br/app/index.php?objeto=" + code
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	o, ok, err := orders.get(ctx, orderID)
	if err != nil || !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
		return
	}
	if err := orders.setTracking(ctx, orderID, code, url, o.LabelURL, o.SuperfreteID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "persist tracking failed"})
		return
	}
	if err := orders.setStatus(ctx, orderID, "shipped"); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"orderId":      o.ID,
			"status":       o.Status,
			"trackingCode": code,
			"trackingUrl":  url,
			"warning":      "tracking saved but status flip failed",
		})
		return
	}
	if h := labelSuccessHook; h != nil {
		h(orderID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orderId":      orderID,
		"status":       "shipped",
		"trackingCode": code,
		"trackingUrl":  url,
	})
}
