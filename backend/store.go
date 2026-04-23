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
	ID              string
	Name            string
	Email           string
	Address         string
	Zip             string
	PaymentMethod   string
	Status          string
	TotalCents      int
	ShippingCents   int
	AmountCents     int
	ShippingSvcID   int
	ShippingSvcName string
	PaymentIntentID string
	TrackingCode    string
	TrackingURL     string
	LabelURL        string
	SuperfreteID    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Items           []orderItem
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
		id, name, email, address, zip, payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
		created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		o.ID, o.Name, o.Email, o.Address, o.Zip, o.PaymentMethod, o.Status,
		o.TotalCents, o.ShippingCents, o.AmountCents,
		nullableInt(o.ShippingSvcID), nullableStr(o.ShippingSvcName),
		nullableStr(o.PaymentIntentID), nullableStr(o.TrackingCode),
		nullableStr(o.TrackingURL), nullableStr(o.LabelURL), nullableStr(o.SuperfreteID),
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
		id, name, email, address, zip, payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
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
		id, name, email, address, zip, payment_method, status,
		total_cents, shipping_cents, amount_cents,
		shipping_service_id, shipping_service_name,
		payment_intent_id, tracking_code, tracking_url, label_url, superfrete_order_id,
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
func (s *orderStore) setTracking(ctx context.Context, orderID, code, url, labelURL, superfreteID string) error {
	_, err := s.db.ExecContext(ctx, rb(`UPDATE orders SET
		tracking_code = ?, tracking_url = ?, label_url = ?, superfrete_order_id = ?, updated_at = ?
		WHERE id = ?`),
		nullableStr(code), nullableStr(url), nullableStr(labelURL),
		nullableStr(superfreteID), time.Now().UTC(), orderID,
	)
	if err != nil {
		return fmt.Errorf("set tracking: %w", err)
	}
	return nil
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
	var shipName, piID, code, url, labelURL, supID sql.NullString
	err := row.Scan(
		&o.ID, &o.Name, &o.Email, &o.Address, &o.Zip,
		&o.PaymentMethod, &o.Status,
		&o.TotalCents, &o.ShippingCents, &o.AmountCents,
		&shipID, &shipName,
		&piID, &code, &url, &labelURL, &supID,
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
