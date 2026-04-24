package main

// DB-backed CRUD for the `coupons` table.
//
// couponsStore is parallel to orderStore / productsStore: handlers
// talk to it, it talks to *sql.DB. Every query is parameterised; the
// code column is normalized by handlers before it ever reaches here.
//
// getForUpdateTx + incrementUsedTx accept a *sql.Tx so checkout can
// validate and increment the counter inside the same transaction that
// creates the order, making the use counter exactly consistent with
// persisted orders.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var errCouponNotFound = errors.New("coupon not found")

type couponsStore struct {
	db *sql.DB
}

func newCouponsStore(db *sql.DB) *couponsStore {
	return &couponsStore{db: db}
}

func (s *couponsStore) list(ctx context.Context) ([]Coupon, error) {
	rows, err := s.db.QueryContext(ctx, rb(`SELECT
		code, kind, value, min_subtotal_cents, max_uses, used_count,
		starts_at, expires_at, active, note, created_at, updated_at
	FROM coupons ORDER BY created_at DESC`))
	if err != nil {
		return nil, fmt.Errorf("list coupons: %w", err)
	}
	defer rows.Close()
	var out []Coupon
	for rows.Next() {
		c, err := scanCoupon(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *couponsStore) get(ctx context.Context, code string) (*Coupon, error) {
	row := s.db.QueryRowContext(ctx, rb(`SELECT
		code, kind, value, min_subtotal_cents, max_uses, used_count,
		starts_at, expires_at, active, note, created_at, updated_at
	FROM coupons WHERE code = ?`), code)
	c, err := scanCoupon(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errCouponNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get coupon: %w", err)
	}
	return c, nil
}

// upsert creates or overwrites a coupon. used_count is preserved on
// update so the admin can safely edit e.g. the discount amount without
// resetting the usage counter.
func (s *couponsStore) upsert(ctx context.Context, c *Coupon) error {
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, rb(`INSERT INTO coupons(
		code, kind, value, min_subtotal_cents, max_uses, used_count,
		starts_at, expires_at, active, note, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(code) DO UPDATE SET
		kind=excluded.kind,
		value=excluded.value,
		min_subtotal_cents=excluded.min_subtotal_cents,
		max_uses=excluded.max_uses,
		starts_at=excluded.starts_at,
		expires_at=excluded.expires_at,
		active=excluded.active,
		note=excluded.note,
		updated_at=excluded.updated_at`),
		c.Code, string(c.Kind), c.Value, c.MinSubtotalCents, c.MaxUses, c.UsedCount,
		nullableTime(c.StartsAt), nullableTime(c.ExpiresAt),
		boolToDB(c.Active), c.Note, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert coupon: %w", err)
	}
	return nil
}

func (s *couponsStore) delete(ctx context.Context, code string) error {
	res, err := s.db.ExecContext(ctx, rb(`DELETE FROM coupons WHERE code = ?`), code)
	if err != nil {
		return fmt.Errorf("delete coupon: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errCouponNotFound
	}
	return nil
}

// getForUpdateTx loads a coupon inside the given transaction, taking a
// row-level lock where the driver supports it (Postgres `FOR UPDATE`).
// SQLite has no row-level locking but begin-exclusive tx serializes
// anyway.
func (s *couponsStore) getForUpdateTx(ctx context.Context, tx *sql.Tx, code string) (*Coupon, error) {
	q := `SELECT
		code, kind, value, min_subtotal_cents, max_uses, used_count,
		starts_at, expires_at, active, note, created_at, updated_at
	FROM coupons WHERE code = ?`
	if currentDialect == dialectPostgres {
		q += ` FOR UPDATE`
	}
	row := tx.QueryRowContext(ctx, rb(q), code)
	c, err := scanCoupon(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errCouponNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get coupon for update: %w", err)
	}
	return c, nil
}

// incrementUsedTx bumps used_count by 1 inside the given transaction.
// Returns errCouponNotFound if the code disappeared between validation
// and this call (extremely unlikely but tx-safe).
func (s *couponsStore) incrementUsedTx(ctx context.Context, tx *sql.Tx, code string) error {
	res, err := tx.ExecContext(ctx, rb(
		`UPDATE coupons SET used_count = used_count + 1, updated_at = ? WHERE code = ?`,
	), time.Now().UTC(), code)
	if err != nil {
		return fmt.Errorf("increment coupon: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errCouponNotFound
	}
	return nil
}

// scanCoupon reads one row from Query/QueryRow. Works for both *sql.Row
// and *sql.Rows.
func scanCoupon(row interface{ Scan(...any) error }) (*Coupon, error) {
	var c Coupon
	var startsAt, expiresAt sql.NullTime
	var active any
	if err := row.Scan(
		&c.Code, &c.Kind, &c.Value, &c.MinSubtotalCents, &c.MaxUses, &c.UsedCount,
		&startsAt, &expiresAt, &active, &c.Note, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if startsAt.Valid {
		t := startsAt.Time.UTC()
		c.StartsAt = &t
	}
	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		c.ExpiresAt = &t
	}
	c.Active = scanBool(active)
	return &c, nil
}

func nullableTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC()
}

// boolToDB encodes a bool in the way the underlying dialect expects:
// SQLite uses INTEGER 0/1, Postgres uses real BOOLEAN.
func boolToDB(v bool) any {
	if currentDialect == dialectPostgres {
		return v
	}
	if v {
		return 1
	}
	return 0
}

// scanBool decodes whatever the driver gave back for a boolean column.
// SQLite returns int64 (0 or 1), Postgres returns bool; driver-specific
// byte slices are handled defensively.
func scanBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	case []byte:
		return len(x) > 0 && x[0] != '0' && x[0] != 0
	case string:
		return x == "true" || x == "1" || x == "t"
	default:
		return false
	}
}
