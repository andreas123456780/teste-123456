package main

// SQLite-backed order persistence.
//
// orderStore is the single read/write surface for the orders + order_items
// + payment_events tables. Handlers talk to it exclusively — nothing
// below the handler layer touches *sql.DB directly.
//
// All operations run in short-lived contexts (bounded at the call site);
// queries are parameterised to defeat SQL injection; string inputs are
// already validated at the HTTP boundary before landing here.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// pendingOrder is the canonical in-memory shape for an order that has
// been created (pending_payment) or transitioned since (paid, failed,
// shipped). We persist every field below plus the per-line items.
type pendingOrder struct {
	ID                  string
	Name                string
	Email               string
	Address             string
	Zip                 string
	// Recipient fields required by SuperFrete. Populated for orders
	// placed after the checkout form redesign; older orders default to
	// empty strings and rely on ViaCEP / operator overrides at retry
	// time. All values are plain strings to match the Postgres schema
	// (TEXT NOT NULL DEFAULT '').
	Document            string // CPF (11) or CNPJ (14), digits only stored
	AddressNumber       string
	AddressComplement   string
	District            string
	City                string
	State               string // UF, uppercase
	PaymentMethod       string
	Status              string
	TotalCents          int
	ShippingCents       int
	AmountCents         int
	ShippingSvcID       int
	ShippingSvcName     string
	PaymentIntentID     string
	TrackingCode        string
	TrackingURL         string
	LabelURL            string
	SuperfreteID        string
	CouponCode          string
	DiscountCents       int
	// UserID is the userAccount.id that placed the order (PR C). Empty
	// string for anonymous/legacy orders; the /api/account/orders
	// endpoint handles those by matching on email_lower.
	UserID              string
	TrackingAttempts    int
	TrackingLastError   string
	TrackingAttemptedAt time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Items               []orderItem
}

type orderItem struct {
	LineNo         int
	ProductID      string
	ProductName    string
	Size           string
	Color          string
	Quantity       int
	UnitPriceCents int
}

// orderStore owns the DB-backed CRUD for orders. The struct is safe for
// concurrent use (sql.DB handles its own pool locking).
type orderStore struct {
	db *sql.DB
}

func newOrderStore(db *sql.DB) *orderStore {
	return &orderStore{db: db}
}

// create inserts a brand-new order (and its items) in a single tx.
// Returns a wrapped error on failure; callers should treat errors as
// unrecoverable (500) — the DB is the source of truth for checkout.
func (s *orderStore) create(ctx context.Context, o *pendingOrder) error {
	if o.ID == "" {
		return errors.New("order id required")
	}
	o.CreatedAt = nonZero(o.CreatedAt, time.Now().UTC())
	o.UpdatedAt = o.CreatedAt
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, rb(`INSERT INTO orders(
		id, name, email, address, zip,
		document, address_number, address_complement, district, city, state,
		payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
		coupon_code, discount_cents, user_id,
		created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		o.ID, o.Name, o.Email, o.Address, o.Zip,
		o.Document, o.AddressNumber, o.AddressComplement, o.District, o.City, o.State,
		o.PaymentMethod, o.Status,
		o.TotalCents, o.ShippingCents, o.AmountCents,
		nullableInt(o.ShippingSvcID), nullableStr(o.ShippingSvcName),
		nullableStr(o.PaymentIntentID), nullableStr(o.TrackingCode),
		nullableStr(o.TrackingURL), nullableStr(o.LabelURL), nullableStr(o.SuperfreteID),
		nullableStr(o.CouponCode), o.DiscountCents, nullableStr(o.UserID),
		o.CreatedAt, o.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	for i, it := range o.Items {
		_, err = tx.ExecContext(ctx, rb(`INSERT INTO order_items(
			order_id, line_no, product_id, product_name, size, color, quantity, unit_price_cents
		) VALUES (?,?,?,?,?,?,?,?)`),
			o.ID, i+1, it.ProductID, it.ProductName,
			nullableStr(it.Size), nullableStr(it.Color),
			it.Quantity, it.UnitPriceCents,
		)
		if err != nil {
			return fmt.Errorf("insert item %d: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// get loads an order (with items) by id. Returns (nil, false, nil) if
// not found so callers don't need to distinguish ErrNoRows.
func (s *orderStore) get(ctx context.Context, id string) (*pendingOrder, bool, error) {
	row := s.db.QueryRowContext(ctx, rb(`SELECT
		id, name, email, address, zip,
		document, address_number, address_complement, district, city, state,
		payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
		coupon_code, discount_cents, user_id,
		tracking_attempts, tracking_last_error, tracking_attempted_at,
		created_at, updated_at
	FROM orders WHERE id = ?`), id)
	o, err := scanOrder(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get order: %w", err)
	}
	if err := s.loadItems(ctx, o); err != nil {
		return nil, false, err
	}
	return o, true, nil
}

// byPaymentIntent returns the order whose payment_intent_id matches piID.
func (s *orderStore) byPaymentIntent(ctx context.Context, piID string) (*pendingOrder, bool, error) {
	if piID == "" {
		return nil, false, nil
	}
	row := s.db.QueryRowContext(ctx, rb(`SELECT
		id, name, email, address, zip,
		document, address_number, address_complement, district, city, state,
		payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
		coupon_code, discount_cents, user_id,
		tracking_attempts, tracking_last_error, tracking_attempted_at,
		created_at, updated_at
	FROM orders WHERE payment_intent_id = ? LIMIT 1`), piID)
	o, err := scanOrder(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("by pi: %w", err)
	}
	if err := s.loadItems(ctx, o); err != nil {
		return nil, false, err
	}
	return o, true, nil
}

// setPaymentIntent attaches a PaymentIntent id to an existing order.
func (s *orderStore) setPaymentIntent(ctx context.Context, orderID, piID string) error {
	_, err := s.db.ExecContext(ctx,
		rb(`UPDATE orders SET payment_intent_id = ?, updated_at = ? WHERE id = ?`),
		piID, time.Now().UTC(), orderID,
	)
	if err != nil {
		return fmt.Errorf("set pi: %w", err)
	}
	return nil
}

// setStatus transitions an order to a new lifecycle status. The webhook
// calls this with "paid" / "failed"; the SuperFrete label job with
// "shipped" when the label is cut.
func (s *orderStore) setStatus(ctx context.Context, orderID, status string) error {
	_, err := s.db.ExecContext(ctx,
		rb(`UPDATE orders SET status = ?, updated_at = ? WHERE id = ?`),
		status, time.Now().UTC(), orderID,
	)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	return nil
}

// setTracking persists the SuperFrete-generated tracking metadata for an
// order. Called from the webhook after a successful label generation.
// On success we also clear any prior failure bookkeeping so the row
// stops appearing in /api/admin/orders/pending-labels.
func (s *orderStore) setTracking(ctx context.Context, orderID, code, url, labelURL, superfreteID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, rb(`UPDATE orders SET
		tracking_code = ?, tracking_url = ?, label_url = ?, superfrete_order_id = ?,
		tracking_last_error = NULL, tracking_attempted_at = ?,
		updated_at = ?
		WHERE id = ?`),
		nullableStr(code), nullableStr(url), nullableStr(labelURL),
		nullableStr(superfreteID), now, now, orderID,
	)
	if err != nil {
		return fmt.Errorf("set tracking: %w", err)
	}
	return nil
}

// markAwaitingShipment is the cart-only mode counterpart of
// setTracking. It persists the SuperFrete cart id (so the admin can
// cross-reference the order in the SuperFrete dashboard), flips the
// order status to "awaiting_shipment", and clears any prior failure
// bookkeeping. No tracking_code is set — that arrives only after the
// operator pays the label via the SuperFrete app.
func (s *orderStore) markAwaitingShipment(ctx context.Context, orderID, superfreteID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, rb(`UPDATE orders SET
		status = 'awaiting_shipment',
		superfrete_order_id = ?,
		tracking_last_error = NULL,
		tracking_attempted_at = ?,
		updated_at = ?
		WHERE id = ?`),
		nullableStr(superfreteID), now, now, orderID,
	)
	if err != nil {
		return fmt.Errorf("mark awaiting shipment: %w", err)
	}
	return nil
}

// recordTrackingFailure increments the per-order attempt counter and
// stores the latest error so the admin UI can show why an order is
// stuck. The cron-driven retry path uses tracking_attempts +
// tracking_attempted_at to compute backoff.
func (s *orderStore) recordTrackingFailure(ctx context.Context, orderID, errMsg string) error {
	const maxErr = 500
	if len(errMsg) > maxErr {
		errMsg = errMsg[:maxErr]
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, rb(`UPDATE orders SET
		tracking_attempts = tracking_attempts + 1,
		tracking_last_error = ?,
		tracking_attempted_at = ?,
		updated_at = ?
		WHERE id = ?`),
		nullableStr(errMsg), now, now, orderID,
	)
	if err != nil {
		return fmt.Errorf("record tracking failure: %w", err)
	}
	return nil
}

// addItem appends a new order_items row to an existing order in a
// single transaction. When `bumpTotals` is true the order's
// total_cents and amount_cents are increased by quantity*unitPrice —
// the cart total moves up. When false (gift / cortesia) only the row
// is inserted, totals stay frozen. Returns the line_no assigned to the
// new row so callers can reflect it in the response.
//
// Used by the admin "Adicionar item ao pedido" panel: a Pix order
// reached the operator on WhatsApp who agreed to throw in a freebie or
// upsell another shirt while the buyer is still on the line.
func (s *orderStore) addItem(ctx context.Context, orderID string, item orderItem, bumpTotals bool) (int, error) {
	if item.Quantity <= 0 {
		return 0, errors.New("quantity must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextLine int
	if err := tx.QueryRowContext(ctx, rb(`SELECT COALESCE(MAX(line_no), 0) + 1
		FROM order_items WHERE order_id = ?`), orderID).Scan(&nextLine); err != nil {
		return 0, fmt.Errorf("next line_no: %w", err)
	}

	if _, err := tx.ExecContext(ctx, rb(`INSERT INTO order_items(
		order_id, line_no, product_id, product_name, size, color, quantity, unit_price_cents
	) VALUES (?,?,?,?,?,?,?,?)`),
		orderID, nextLine, item.ProductID, item.ProductName,
		nullableStr(item.Size), nullableStr(item.Color),
		item.Quantity, item.UnitPriceCents,
	); err != nil {
		return 0, fmt.Errorf("insert item: %w", err)
	}

	now := time.Now().UTC()
	if bumpTotals {
		delta := item.UnitPriceCents * item.Quantity
		if _, err := tx.ExecContext(ctx, rb(`UPDATE orders SET
			total_cents  = total_cents  + ?,
			amount_cents = amount_cents + ?,
			updated_at   = ?
			WHERE id = ?`), delta, delta, now, orderID); err != nil {
			return 0, fmt.Errorf("bump totals: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, rb(`UPDATE orders SET updated_at = ? WHERE id = ?`),
			now, orderID); err != nil {
			return 0, fmt.Errorf("touch updated_at: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return nextLine, nil
}

// listPendingLabels returns paid orders that still don't have a
// tracking_code, ordered by the next retry deadline. The caller passes
// a cutoff (now() minus the smallest backoff) so freshly-attempted
// rows are skipped, plus a maxAttempts cap so we don't burn cycles on
// permanently broken orders. Used by the cron job and by the admin
// listing endpoint (with a generous cutoff).
func (s *orderStore) listPendingLabels(ctx context.Context, cutoff time.Time, maxAttempts, limit int) ([]*pendingOrder, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx, rb(`SELECT
		id, name, email, address, zip,
		document, address_number, address_complement, district, city, state,
		payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
		coupon_code, discount_cents, user_id,
		tracking_attempts, tracking_last_error, tracking_attempted_at,
		created_at, updated_at
	FROM orders
	WHERE status = 'paid'
	  AND tracking_code IS NULL
	  AND tracking_attempts < ?
	  AND (tracking_attempted_at IS NULL OR tracking_attempted_at <= ?)
	ORDER BY tracking_attempts ASC, created_at ASC
	LIMIT ?`), maxAttempts, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending labels: %w", err)
	}
	defer rows.Close()
	var out []*pendingOrder
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending label: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending labels: %w", err)
	}
	for _, o := range out {
		if err := s.loadItems(ctx, o); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// listOrders returns the most recent orders ordered by created_at
// descending. The admin dashboard uses it to show a paginated list of
// every order (not only the stuck ones). An optional status filter
// lets the UI split "pendentes" vs "pagos" vs "enviados" without
// loading everything and filtering client-side.
func (s *orderStore) listOrders(ctx context.Context, status string, limit, offset int) ([]*pendingOrder, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	baseCols := `id, name, email, address, zip,
		document, address_number, address_complement, district, city, state,
		payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
		coupon_code, discount_cents, user_id,
		tracking_attempts, tracking_last_error, tracking_attempted_at,
		created_at, updated_at`
	var (
		rows *sql.Rows
		err  error
	)
	if status == "" {
		rows, err = s.db.QueryContext(ctx, rb(`SELECT `+baseCols+`
			FROM orders ORDER BY created_at DESC LIMIT ? OFFSET ?`),
			limit, offset)
	} else {
		rows, err = s.db.QueryContext(ctx, rb(`SELECT `+baseCols+`
			FROM orders WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`),
			status, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()
	var out []*pendingOrder
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orders: %w", err)
	}
	for _, o := range out {
		if err := s.loadItems(ctx, o); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// deleteOrder removes an order + its line items + its payment_events
// rows. Used by the admin "apagar pedido" button for cleaning up test
// orders. Order rows with real payments should never be deleted —
// the handler is responsible for that gating; the store just
// enforces cascade cleanup to keep FK-like invariants when running
// against SQLite (where FKs are enabled by PRAGMA but tests never
// set them).
func (s *orderStore) deleteOrder(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, rb(`DELETE FROM order_items WHERE order_id = ?`), id); err != nil {
		return fmt.Errorf("delete items: %w", err)
	}
	if _, err := tx.ExecContext(ctx, rb(`DELETE FROM payment_events WHERE order_id = ?`), id); err != nil {
		return fmt.Errorf("delete events: %w", err)
	}
	res, err := tx.ExecContext(ctx, rb(`DELETE FROM orders WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete order: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errOrderNotFound
	}
	return tx.Commit()
}

// errOrderNotFound is returned by deleteOrder when the order id
// doesn't match any row. The handler translates it to a 404.
var errOrderNotFound = errors.New("order not found")

// listByUser returns every order belonging to a given customer. Uses
// a UNION of two predicates:
//   - orders.user_id = userID          (canonical link)
//   - orders.email   = emailLower      (legacy match for orders
//                                       placed before PR C)
// Both arms filter on status so pending_payment orders (where the
// customer never completed Stripe) don't leak into the history. The
// cap (50) is conservative — the /minha-conta UI doesn't paginate.
func (s *orderStore) listByUser(ctx context.Context, userID, emailLower string) ([]*pendingOrder, error) {
	if userID == "" && emailLower == "" {
		return nil, nil
	}
	baseCols := `id, name, email, address, zip,
		document, address_number, address_complement, district, city, state,
		payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
		coupon_code, discount_cents, user_id,
		tracking_attempts, tracking_last_error, tracking_attempted_at,
		created_at, updated_at`
	// The WHERE clause is built dynamically so we don't have to pass
	// a placeholder for the email when userID alone is enough (and
	// vice versa). Lowercasing is done by the caller so we keep this
	// query case-sensitive-but-normalized.
	where := `WHERE status != 'pending_payment' AND (`
	args := []any{}
	parts := []string{}
	if userID != "" {
		parts = append(parts, "user_id = ?")
		args = append(args, userID)
	}
	if emailLower != "" {
		parts = append(parts, "LOWER(email) = ?")
		args = append(args, emailLower)
	}
	where += joinOr(parts) + `)`
	rows, err := s.db.QueryContext(ctx, rb(`SELECT `+baseCols+`
		FROM orders `+where+`
		ORDER BY created_at DESC LIMIT 50`), args...)
	if err != nil {
		return nil, fmt.Errorf("list by user: %w", err)
	}
	defer rows.Close()
	var out []*pendingOrder
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, o := range out {
		if err := s.loadItems(ctx, o); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// linkOrdersByEmail retroactively attaches user_id to every order
// with a matching email_lower that doesn't already have one. Called
// on signup/login so PR C's /minha-conta page shows history for
// purchases made before the customer had an account.
func (s *orderStore) linkOrdersByEmail(ctx context.Context, userID, emailLower string) error {
	if userID == "" || emailLower == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, rb(
		`UPDATE orders SET user_id = ?, updated_at = ?
		 WHERE (user_id IS NULL OR user_id = '') AND LOWER(email) = ?`),
		userID, time.Now().UTC(), emailLower,
	)
	if err != nil {
		return fmt.Errorf("link legacy orders: %w", err)
	}
	return nil
}

// joinOr is strings.Join(parts, " OR ") without importing strings
// just for that — store.go is already import-lean.
func joinOr(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " OR "
		}
		out += p
	}
	return out
}

// recordEvent appends a row to payment_events. The payload is kept as
// raw JSON text so we can replay/audit it later without re-fetching
// from Stripe. Errors are returned but callers typically log-and-continue.
func (s *orderStore) recordEvent(ctx context.Context, orderID, eventType, piID string, payload any) error {
	var raw string
	if payload != nil {
		b, err := json.Marshal(payload)
		if err == nil {
			// Cap stored payload so a malicious sender can't fill the DB.
			if len(b) > 1<<17 {
				b = b[:1<<17]
			}
			raw = string(b)
		}
	}
	_, err := s.db.ExecContext(ctx, rb(`INSERT INTO payment_events(
		order_id, event_type, payment_intent_id, raw_payload, created_at
	) VALUES (?,?,?,?,?)`),
		nullableStr(orderID), eventType, nullableStr(piID),
		nullableStr(raw), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}
	return nil
}

func (s *orderStore) loadItems(ctx context.Context, o *pendingOrder) error {
	rows, err := s.db.QueryContext(ctx, rb(`SELECT
		line_no, product_id, product_name, size, color, quantity, unit_price_cents
	FROM order_items WHERE order_id = ? ORDER BY line_no`), o.ID)
	if err != nil {
		return fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it orderItem
		var size, color sql.NullString
		if err := rows.Scan(&it.LineNo, &it.ProductID, &it.ProductName,
			&size, &color, &it.Quantity, &it.UnitPriceCents); err != nil {
			return fmt.Errorf("scan item: %w", err)
		}
		it.Size = size.String
		it.Color = color.String
		o.Items = append(o.Items, it)
	}
	return rows.Err()
}

// scanOrder is shared by get and byPaymentIntent.
func scanOrder(row interface{ Scan(...any) error }) (*pendingOrder, error) {
	var o pendingOrder
	var shipID sql.NullInt64
	var shipName, piID, code, url, labelURL, supID, couponCode, lastErr, userID sql.NullString
	var attemptedAt sql.NullTime
	err := row.Scan(
		&o.ID, &o.Name, &o.Email, &o.Address, &o.Zip,
		&o.Document, &o.AddressNumber, &o.AddressComplement,
		&o.District, &o.City, &o.State,
		&o.PaymentMethod, &o.Status,
		&o.TotalCents, &o.ShippingCents, &o.AmountCents,
		&shipID, &shipName,
		&piID, &code, &url, &labelURL, &supID,
		&couponCode, &o.DiscountCents, &userID,
		&o.TrackingAttempts, &lastErr, &attemptedAt,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if shipID.Valid {
		o.ShippingSvcID = int(shipID.Int64)
	}
	o.ShippingSvcName = shipName.String
	o.PaymentIntentID = piID.String
	o.TrackingCode = code.String
	o.TrackingURL = url.String
	o.LabelURL = labelURL.String
	o.SuperfreteID = supID.String
	o.CouponCode = couponCode.String
	o.UserID = userID.String
	o.TrackingLastError = lastErr.String
	if attemptedAt.Valid {
		o.TrackingAttemptedAt = attemptedAt.Time
	}
	return &o, nil
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt(i int) any {
	if i == 0 {
		return nil
	}
	return i
}

func nonZero(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}
