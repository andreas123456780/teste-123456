package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// productsStore is the database-backed catalog. It exposes read methods
// consumed by the public /api/products endpoints and write methods
// protected by the admin token.
//
// The store never caches: SQLite reads are cheap (typically <1ms) and
// keeping the path simple avoids the cache-coherency bugs that tend to
// creep in when products can be edited from an admin UI.
type productsStore struct {
	db *sql.DB
}

func newProductsStore(db *sql.DB) *productsStore {
	return &productsStore{db: db}
}

var errProductNotFound = errors.New("product not found")

// listPublic returns the catalog as the storefront sees it: hidden rows
// are excluded and the order is deterministic (sort_order asc, id asc).
// An optional category filter is applied case-insensitively.
func (s *productsStore) listPublic(ctx context.Context, category string) ([]Product, error) {
	const baseQ = `SELECT id, name, description, price_cents, pix_price_cents, category, image, back_image,
		colors_json, sizes_json, tags_json, stock
		FROM products WHERE hidden = 0`
	q := baseQ + " ORDER BY sort_order ASC, id ASC"
	args := []any{}
	if category != "" {
		q = baseQ + " AND LOWER(category) = LOWER(?) ORDER BY sort_order ASC, id ASC"
		args = append(args, category)
	}
	return s.query(ctx, rb(q), args...)
}

// listAdmin returns every product, hidden or not. Admin only.
func (s *productsStore) listAdmin(ctx context.Context) ([]Product, error) {
	const q = `SELECT id, name, description, price_cents, pix_price_cents, category, image, back_image,
		colors_json, sizes_json, tags_json, stock
		FROM products ORDER BY sort_order ASC, id ASC`
	return s.query(ctx, rb(q))
}

func (s *productsStore) get(ctx context.Context, id string) (*Product, error) {
	const q = `SELECT id, name, description, price_cents, pix_price_cents, category, image, back_image,
		colors_json, sizes_json, tags_json, stock FROM products WHERE id = ?`
	row := s.db.QueryRowContext(ctx, rb(q), id)
	p, err := scanProduct(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errProductNotFound
	}
	return p, err
}

func (s *productsStore) upsert(ctx context.Context, p *Product, hidden bool, sortOrder int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	colors, _ := json.Marshal(stringsOrEmpty(p.Colors))
	sizes, _ := json.Marshal(stringsOrEmpty(p.Sizes))
	tags, _ := json.Marshal(stringsOrEmpty(p.Tags))
	const q = `INSERT INTO products
		(id, name, description, price_cents, pix_price_cents, category, image, back_image,
		 colors_json, sizes_json, tags_json, stock, hidden, sort_order, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			price_cents = excluded.price_cents,
			pix_price_cents = excluded.pix_price_cents,
			category = excluded.category,
			image = excluded.image,
			back_image = excluded.back_image,
			colors_json = excluded.colors_json,
			sizes_json = excluded.sizes_json,
			tags_json = excluded.tags_json,
			stock = excluded.stock,
			hidden = excluded.hidden,
			sort_order = excluded.sort_order,
			updated_at = excluded.updated_at`
	_, err := s.db.ExecContext(ctx, rb(q),
		p.ID, p.Name, p.Description, p.PriceCents, p.PixPriceCents, p.Category,
		p.Image, p.BackImage, string(colors), string(sizes), string(tags),
		p.Stock, boolToInt(hidden), sortOrder, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert product: %w", err)
	}
	return nil
}

func (s *productsStore) delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, rb(`DELETE FROM products WHERE id = ?`), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errProductNotFound
	}
	return nil
}

// decrementStock reduces stock atomically, never going below zero. The
// return value is the number of units that could NOT be fulfilled
// (0 = the whole quantity was decremented). Callers log when > 0 so
// operators can follow up.
func (s *productsStore) decrementStock(ctx context.Context, id string, qty int) (int, error) {
	if qty <= 0 {
		return 0, nil
	}
	// Atomic: compute min(stock, qty), subtract that, return remainder.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return qty, err
	}
	defer func() { _ = tx.Rollback() }()
	var stock int
	row := tx.QueryRowContext(ctx, rb(`SELECT stock FROM products WHERE id = ?`), id)
	if err := row.Scan(&stock); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return qty, nil
		}
		return qty, err
	}
	applied := qty
	if applied > stock {
		applied = stock
	}
	if applied > 0 {
		if _, err := tx.ExecContext(ctx, rb(`UPDATE products SET stock = stock - ?, updated_at = ? WHERE id = ?`),
			applied, time.Now().UTC().Format(time.RFC3339), id); err != nil {
			return qty, err
		}
	}
	if err := tx.Commit(); err != nil {
		return qty, err
	}
	return qty - applied, nil
}

func (s *productsStore) query(ctx context.Context, q string, args ...any) ([]Product, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Product, 0, 16)
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanProduct(row scannable) (*Product, error) {
	var p Product
	var colors, sizes, tags string
	if err := row.Scan(
		&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.PixPriceCents, &p.Category,
		&p.Image, &p.BackImage, &colors, &sizes, &tags, &p.Stock,
	); err != nil {
		return nil, err
	}
	p.Colors = parseStringList(colors)
	p.Sizes = parseStringList(sizes)
	p.Tags = parseStringList(tags)
	return &p, nil
}

func parseStringList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return []string{}
	}
	return out
}

func stringsOrEmpty(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// seedProductsIfEmpty copies the in-code catalog into the DB when the
// products table has zero rows. This lets a fresh deploy come up with a
// populated storefront without any manual admin bootstrap step. Once
// the admin edits any row, this seed is never re-applied.
func seedProductsIfEmpty(ctx context.Context, s *productsStore, seed []Product) error {
	rows, err := s.listAdmin(ctx)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		return nil
	}
	for i, p := range seed {
		pp := p
		if err := s.upsert(ctx, &pp, false, i); err != nil {
			return err
		}
	}
	return nil
}
