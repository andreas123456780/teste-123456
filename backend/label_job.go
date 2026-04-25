package main

// SuperFrete label generation.
//
// Original design: the Stripe webhook handler called Enqueue on a
// goroutine-backed labelWorker. That goroutine survives only as long
// as the OS process — fine on Fly.io, broken on Vercel/Cloud Run/any
// serverless platform that suspends the function as soon as the HTTP
// response is flushed.
//
// New design: every code path that wants a label calls runLabelJob
// synchronously. The function is idempotent (skips orders that
// already have a tracking_code) and bookkeeping (attempts + last
// error) lives in the orders table so a cron-driven worker can pick
// up failures with backoff. See internal_jobs.go for that worker.

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"
)

// syncLabelDispatcher implements labelEnqueuer by running runLabelJob
// synchronously inside Enqueue. The webhook handler thus blocks on the
// SuperFrete pipeline before returning 200 to Stripe — which is the
// only design that survives serverless platforms (Vercel, Cloud Run)
// where goroutines die with the function. Errors are recorded against
// the order and a fallback cron sweep retries with backoff.
type syncLabelDispatcher struct {
	orders  *orderStore
	ship    *shippingClient
	viacep  *viaCepClient
	timeout time.Duration
}

func newSyncLabelDispatcher(orders *orderStore, ship *shippingClient, timeout time.Duration) *syncLabelDispatcher {
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	return &syncLabelDispatcher{orders: orders, ship: ship, viacep: newViaCepClient(), timeout: timeout}
}

// labelOverrides carries operator-provided overrides for a single
// runLabelJob invocation. Used by the admin retry endpoint so the
// operator can patch a bad recipient name (or manually-entered address
// line) without having to edit the underlying order row first.
type labelOverrides struct {
	Name     string
	Address  string
	Document string         // CPF or CNPJ, digits only preferred; SuperFrete requires this on most accounts
	Details  addressDetails // empty fields fall back to ViaCEP
}

// Enqueue blocks until runLabelJob returns or the per-order timeout
// fires. The webhook handler calls this from the request goroutine
// and tolerates the latency: SuperFrete is ~5-10s on a good day, well
// inside Stripe's 30s webhook budget.
func (d *syncLabelDispatcher) Enqueue(orderID string) {
	if d == nil || d.ship == nil || d.orders == nil || orderID == "" {
		return
	}
	if d.ship.cfg.AccessToken == "" {
		log.Printf("label_job: SuperFrete not configured, skipping order %s", orderID)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	if err := runLabelJob(ctx, d.orders, d.ship, d.viacep, orderID, labelOverrides{}); err != nil {
		log.Printf("label_job: order %s: %v (will retry via cron)", orderID, err)
	}
}

// runLabelJob runs the SuperFrete pipeline for a single order. Safe to
// call multiple times: orders that already have a tracking_code are
// skipped. On failure the error is recorded against the order
// (tracking_attempts + tracking_last_error) so the cron retry loop has
// state to back off against — and the error is returned so the
// caller's logs aren't blank.
func runLabelJob(ctx context.Context, orders *orderStore, ship *shippingClient, viacep *viaCepClient, orderID string, overrides labelOverrides) error {
	if orders == nil {
		return errors.New("label job: order store missing")
	}
	if ship == nil || ship.cfg.AccessToken == "" {
		return errors.New("label job: SuperFrete not configured")
	}
	if orderID == "" {
		return errors.New("label job: empty order id")
	}

	o, ok, err := orders.get(ctx, orderID)
	if err != nil {
		return wrapAndRecord(ctx, orders, orderID, "load order", err)
	}
	if !ok {
		return errors.New("label job: order not found")
	}
	if o.TrackingCode != "" {
		// Already generated — duplicate webhook delivery or cron
		// hitting a row right after a successful run. Idempotent skip.
		return nil
	}
	if o.Status != "paid" {
		// Don't generate labels for orders that aren't paid yet
		// (refunded/failed orders should never call this either, but
		// belt-and-suspenders).
		return nil
	}

	pkgs, err := buildPackagesFromOrder(o)
	if err != nil {
		return wrapAndRecord(ctx, orders, orderID, "build packages", err)
	}
	vol, insurance := mergeVolumes(pkgs)

	name := strings.TrimSpace(overrides.Name)
	if name == "" {
		name = strings.TrimSpace(o.Name)
	}
	address := strings.TrimSpace(overrides.Address)
	if address == "" {
		address = strings.TrimSpace(o.Address)
	}

	details, err := resolveAddressDetails(ctx, viacep, o.Zip, overrides.Details)
	if err != nil {
		return wrapAndRecord(ctx, orders, orderID, "viacep", err)
	}

	to := map[string]any{
		"name":        name,
		"email":       o.Email,
		"address":     address,
		"postal_code": digitsOnly(o.Zip),
	}
	if doc := digitsOnly(overrides.Document); doc != "" {
		to["document"] = doc
	}
	if details.District != "" {
		to["district"] = details.District
	}
	if details.City != "" {
		to["city"] = details.City
	}
	if details.State != "" {
		to["state_abbr"] = details.State
	}
	cart := map[string]any{
		"to":              to,
		"service":         o.ShippingSvcID,
		"products":        buildCartProducts(o),
		"volumes":         []any{vol},
		"insurance_value": insurance,
		"tag":             o.ID,
	}

	sfOrderID, err := ship.AddToCart(ctx, cart)
	if err != nil {
		return wrapAndRecord(ctx, orders, orderID, "addtocart", err)
	}
	if _, err := ship.Checkout(ctx, []string{sfOrderID}); err != nil {
		return wrapAndRecord(ctx, orders, orderID, "checkout", err)
	}
	if _, err := ship.Generate(ctx, []string{sfOrderID}); err != nil {
		return wrapAndRecord(ctx, orders, orderID, "generate", err)
	}
	printed, err := ship.Print(ctx, []string{sfOrderID}, "private")
	if err != nil {
		return wrapAndRecord(ctx, orders, orderID, "print", err)
	}

	// OrderInfo failures are non-fatal: the label is already paid for
	// and can be reprinted from the SuperFrete dashboard. We log and
	// continue with whatever metadata Print returned.
	info, infoErr := ship.OrderInfo(ctx, sfOrderID)
	if infoErr != nil {
		log.Printf("label_job: order_info(%s): %v (non-fatal)", orderID, infoErr)
	}
	trackingCode, trackingURL := parseTrackingFromInfo(info)
	labelURL := parseLabelURL(printed)

	if err := orders.setTracking(ctx, orderID, trackingCode, trackingURL, labelURL, sfOrderID); err != nil {
		return wrapAndRecord(ctx, orders, orderID, "set tracking", err)
	}
	if err := orders.setStatus(ctx, orderID, "shipped"); err != nil {
		// Tracking is already saved; logging is enough.
		log.Printf("label_job: setStatus(%s): %v", orderID, err)
	}
	log.Printf("label_job: order %s shipped (sf=%s tracking=%s)", orderID, sfOrderID, trackingCode)
	return nil
}

// wrapAndRecord stamps the underlying error onto the order row and
// returns it so the caller can log + propagate. Errors from the record
// step itself are logged but never replace the original failure.
func wrapAndRecord(ctx context.Context, orders *orderStore, orderID, stage string, cause error) error {
	wrapped := errors.New("label_job: " + stage + ": " + cause.Error())
	// Use a short, fresh context for the bookkeeping write so a
	// cancelled parent ctx doesn't keep us from recording the error.
	bookCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := orders.recordTrackingFailure(bookCtx, orderID, wrapped.Error()); err != nil {
		log.Printf("label_job: recordTrackingFailure(%s): %v", orderID, err)
	}
	return wrapped
}

// labelBackoff returns the minimum delay between attempts for a given
// attempt count. We start at 2 minutes and double up to 1 hour. The
// cron worker queries `now - tracking_attempted_at >= labelBackoff` to
// decide whether a stuck order is ready for another try.
func labelBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		return 0
	}
	const base = 2 * time.Minute
	const max = time.Hour
	d := base
	for i := 1; i < attempts && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// maxLabelAttempts caps how many times we retry a single order before
// it stops appearing in the cron sweep. Past this, the admin UI still
// surfaces the order so an operator can fix the data and re-trigger.
const maxLabelAttempts = 8

// resolveAddressDetails merges operator-supplied overrides with a
// ViaCEP lookup against the order's zip. The priority is:
//   1. any field the operator passed explicitly (overrides.Details)
//   2. ViaCEP lookup for anything missing
//   3. leave field blank — SuperFrete will reject, and the caller
//      records the failure for the admin UI
// ViaCEP outages are not fatal when the operator has already supplied
// a complete override; that's the manual-recovery path.
func resolveAddressDetails(ctx context.Context, client *viaCepClient, zip string, overrides addressDetails) (addressDetails, error) {
	out := overrides
	if out.District != "" && out.City != "" && out.State != "" {
		return out, nil
	}
	if client == nil {
		return out, errors.New("address lookup unavailable")
	}
	lookup, err := client.Lookup(ctx, zip)
	if err != nil {
		// If the operator gave us at least city + state we still want
		// to try shipping; district alone is rarely enforced by
		// SuperFrete but we surface the ViaCEP error for observability.
		if out.City != "" && out.State != "" {
			log.Printf("label_job: viacep lookup failed for %s: %v (using overrides)", zip, err)
			return out, nil
		}
		return out, err
	}
	if out.District == "" {
		out.District = lookup.District
	}
	if out.City == "" {
		out.City = lookup.City
	}
	if out.State == "" {
		out.State = lookup.State
	}
	return out, nil
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
			"name":          it.ProductName,
			"quantity":      it.Quantity,
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
		Tracking     string `json:"tracking"`
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
