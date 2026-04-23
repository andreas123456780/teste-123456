package main

// Async SuperFrete label generation.
//
// Stripe webhooks have a 15s delivery budget; the full SuperFrete label
// flow (cart → checkout → generate → print) is ~5-10s of synchronous
// HTTP on a good day, well north of that when SuperFrete is slow. We
// fire-and-forget: the webhook handler enqueues the order id, a single
// worker goroutine drains the channel and runs the steps against a
// fresh context with its own timeout.
//
// If the goroutine falls behind (channel full) we log and drop — the
// admin can manually call /api/shipping/label later. We do NOT retry
// automatically here; the retry policy for SuperFrete is out of scope
// for this service and is better handled by a proper job queue.

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// labelWorker is a labelEnqueuer (see payments.go) backed by a single
// goroutine and a buffered channel.
type labelWorker struct {
	orders *orderStore
	ship   *shippingClient
	queue  chan string
}

// newLabelWorker starts the worker goroutine and returns a handle. When
// ship or orders is nil it returns a no-op worker so the rest of the
// service can boot without SuperFrete configured.
func newLabelWorker(orders *orderStore, ship *shippingClient) *labelWorker {
	w := &labelWorker{
		orders: orders,
		ship:   ship,
		queue:  make(chan string, 256),
	}
	go w.run()
	return w
}

// Enqueue implements labelEnqueuer. Non-blocking; logs if the channel is
// full so operations can alert on backpressure.
func (w *labelWorker) Enqueue(orderID string) {
	if w == nil || w.ship == nil || orderID == "" {
		return
	}
	select {
	case w.queue <- orderID:
	default:
		log.Printf("label_job: queue full, dropping order=%s (manual intervention needed)", orderID)
	}
}

func (w *labelWorker) run() {
	for id := range w.queue {
		w.process(id)
	}
}

// process executes the SuperFrete pipeline for one order. Errors are
// logged with the orderID but otherwise swallowed — the customer already
// paid, retrying forever in a tight loop would only make things worse.
func (w *labelWorker) process(orderID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	o, ok, err := w.orders.get(ctx, orderID)
	if err != nil {
		log.Printf("label_job: get(%s): %v", orderID, err)
		return
	}
	if !ok {
		log.Printf("label_job: order %s not found", orderID)
		return
	}
	if o.TrackingCode != "" {
		// Already generated — likely a duplicate webhook delivery.
		return
	}
	if w.ship.cfg.AccessToken == "" {
		log.Printf("label_job: SuperFrete not configured, skipping order %s", orderID)
		return
	}

	// Build a SuperFrete cart payload from the order. We use the single
	// merged volume (the quote flow already handles multi-item packing)
	// and preserve the service chosen by the customer.
	pkgs, err := buildPackagesFromOrder(o)
	if err != nil {
		log.Printf("label_job: build packages for %s: %v", orderID, err)
		return
	}
	vol, insurance := mergeVolumes(pkgs)
	cart := map[string]any{
		"to": map[string]any{
			"name":            o.Name,
			"email":           o.Email,
			"address":         o.Address,
			"postal_code":     digitsOnly(o.Zip),
		},
		"service":         o.ShippingSvcID,
		"products":        buildCartProducts(o),
		"volumes":         []any{vol},
		"insurance_value": insurance,
		"tag":             o.ID,
	}

	sfOrderID, err := w.ship.AddToCart(ctx, cart)
	if err != nil {
		log.Printf("label_job: AddToCart(%s): %v", orderID, err)
		return
	}
	if _, err := w.ship.Checkout(ctx, []string{sfOrderID}); err != nil {
		log.Printf("label_job: Checkout(%s): %v", orderID, err)
		return
	}
	if _, err := w.ship.Generate(ctx, []string{sfOrderID}); err != nil {
		log.Printf("label_job: Generate(%s): %v", orderID, err)
		return
	}
	printed, err := w.ship.Print(ctx, []string{sfOrderID}, "private")
	if err != nil {
		log.Printf("label_job: Print(%s): %v", orderID, err)
		return
	}

	info, err := w.ship.OrderInfo(ctx, sfOrderID)
	if err != nil {
		log.Printf("label_job: OrderInfo(%s): %v", orderID, err)
	}
	trackingCode, trackingURL := parseTrackingFromInfo(info)
	labelURL := parseLabelURL(printed)

	if err := w.orders.setTracking(ctx, orderID, trackingCode, trackingURL, labelURL, sfOrderID); err != nil {
		log.Printf("label_job: setTracking(%s): %v", orderID, err)
		return
	}
	if err := w.orders.setStatus(ctx, orderID, "shipped"); err != nil {
		log.Printf("label_job: setStatus(%s): %v", orderID, err)
		return
	}
	log.Printf("label_job: order %s shipped (sf=%s tracking=%s)", orderID, sfOrderID, trackingCode)
}

// buildPackagesFromOrder reconstructs the per-product package presets we
// stored in the catalog at quote time. The order_items table is the
// source of truth for quantities.
func buildPackagesFromOrder(o *pendingOrder) ([]ShippingPackage, error) {
	refs := make([]ShippingCartRef, 0, len(o.Items))
	for _, it := range o.Items {
		refs = append(refs, ShippingCartRef{ProductID: it.ProductID, Quantity: it.Quantity})
	}
	return buildPackages(refs)
}

// buildCartProducts produces the `products` array SuperFrete expects in
// /api/v0/cart — one line per distinct SKU.
func buildCartProducts(o *pendingOrder) []map[string]any {
	out := make([]map[string]any, 0, len(o.Items))
	for _, it := range o.Items {
		out = append(out, map[string]any{
			"name":         it.ProductName,
			"quantity":     it.Quantity,
			"unitary_value": float64(it.UnitPriceCents) / 100.0,
		})
	}
	return out
}

// parseTrackingFromInfo pulls the Correios tracking code + URL from a
// /order/info response. Returns empty strings if the response is not in
// the expected shape — the caller will log and move on.
func parseTrackingFromInfo(raw json.RawMessage) (code, url string) {
	if len(raw) == 0 {
		return "", ""
	}
	var shape struct {
		Tracking    string `json:"tracking"`
		TrackingCode string `json:"tracking_code"`
		TrackingURL  string `json:"tracking_url"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return "", ""
	}
	code = strings.TrimSpace(shape.TrackingCode)
	if code == "" {
		code = strings.TrimSpace(shape.Tracking)
	}
	url = strings.TrimSpace(shape.TrackingURL)
	if url == "" && code != "" {
		// Fall back to the public Correios portal.
		url = "https://rastreamento.correios.com.br/app/index.php?objeto=" + code
	}
	return code, url
}

func parseLabelURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Print response shapes observed in the wild:
	//   {"url": "https://..."}
	//   {"link": "https://..."}
	//   ["https://...", ...]
	var shape struct {
		URL  string `json:"url"`
		Link string `json:"link"`
	}
	if err := json.Unmarshal(raw, &shape); err == nil {
		if shape.URL != "" {
			return shape.URL
		}
		if shape.Link != "" {
			return shape.Link
		}
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil && len(list) > 0 {
		return list[0]
	}
	return ""
}
