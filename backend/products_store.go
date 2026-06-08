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
		colors_json, sizes_json, tags_json, stock, stock_by_size, transparent_image
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
		colors_json, sizes_json, tags_json, stock, stock_by_size, transparent_image
		FROM products ORDER BY sort_order ASC, id ASC`
	return s.query(ctx, rb(q))
}

func (s *productsStore) get(ctx context.Context, id string) (*Product, error) {
	const q = `SELECT id, name, description, price_cents, pix_price_cents, category, image, back_image,
		colors_json, sizes_json, tags_json, stock, stock_by_size, transparent_image FROM products WHERE id = ?`
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
	stockBySize, _ := json.Marshal(normalizeStockBySize(p.StockBySize, p.Sizes))
	const q = `INSERT INTO products
		(id, name, description, price_cents, pix_price_cents, category, image, back_image,
		 colors_json, sizes_json, tags_json, stock, stock_by_size, transparent_image, hidden, sort_order, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
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
			stock_by_size = excluded.stock_by_size,
			transparent_image = excluded.transparent_image,
			hidden = excluded.hidden,
			sort_order = excluded.sort_order,
			updated_at = excluded.updated_at`
	_, err := s.db.ExecContext(ctx, rb(q),
		p.ID, p.Name, p.Description, p.PriceCents, p.PixPriceCents, p.Category,
		p.Image, p.BackImage, string(colors), string(sizes), string(tags),
		p.Stock, string(stockBySize), boolToInt(p.TransparentImage), boolToInt(hidden), sortOrder, now, now,
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

// decrementStockBySize reduces both the per-size entry and the legacy
// total column in a single transaction, never going below zero. When
// the product has no stock_by_size entries yet (legacy row) the call
// falls back to decrementing only the total. Returns the number of
// units that couldn't be fulfilled.
func (s *productsStore) decrementStockBySize(ctx context.Context, id, size string, qty int) (int, error) {
	if qty <= 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return qty, err
	}
	defer func() { _ = tx.Rollback() }()
	var total int
	var raw string
	row := tx.QueryRowContext(ctx, rb(`SELECT stock, stock_by_size FROM products WHERE id = ?`), id)
	if err := row.Scan(&total, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return qty, nil
		}
		return qty, err
	}
	m := parseStockBySize(raw)
	// Legacy fallback: if we don't have per-size data yet, only trim
	// the total so we at least honor the overall cap.
	if len(m) == 0 {
		applied := qty
		if applied > total {
			applied = total
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
	available := m[size]
	applied := qty
	if applied > available {
		applied = available
	}
	if applied > 0 {
		m[size] = available - applied
		nextRaw, _ := json.Marshal(m)
		newTotal := total - applied
		if newTotal < 0 {
			newTotal = 0
		}
		if _, err := tx.ExecContext(ctx, rb(`UPDATE products SET stock = ?, stock_by_size = ?, updated_at = ? WHERE id = ?`),
			newTotal, string(nextRaw), time.Now().UTC().Format(time.RFC3339), id); err != nil {
			return qty, err
		}
	}
	if err := tx.Commit(); err != nil {
		return qty, err
	}
	return qty - applied, nil
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
	var colors, sizes, tags, stockBySize string
	var transparent int
	if err := row.Scan(
		&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.PixPriceCents, &p.Category,
		&p.Image, &p.BackImage, &colors, &sizes, &tags, &p.Stock, &stockBySize, &transparent,
	); err != nil {
		return nil, err
	}
	p.Colors = parseStringList(colors)
	p.Sizes = parseStringList(sizes)
	p.Tags = parseStringList(tags)
	p.StockBySize = parseStockBySize(stockBySize)
	p.TransparentImage = transparent != 0
	return &p, nil
}

// parseStockBySize decodes the JSON blob stored in products.stock_by_size.
// An empty or malformed value yields an empty map; callers should handle
// that by falling back to the legacy total stock column.
func parseStockBySize(s string) map[string]int {
	out := map[string]int{}
	if strings.TrimSpace(s) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return map[string]int{}
	}
	return out
}

// normalizeStockBySize returns a copy of in that only contains keys
// present in sizes. Values are clamped to [0, 10_000]. This prevents
// stale entries (e.g. a size that was renamed) from haunting the map.
func normalizeStockBySize(in map[string]int, sizes []string) map[string]int {
	out := make(map[string]int, len(sizes))
	for _, sz := range sizes {
		v := in[sz]
		if v < 0 {
			v = 0
		}
		if v > 10_000 {
			v = 10_000
		}
		out[sz] = v
	}
	return out
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

// seedInsert inserts a product only if no row with that id exists yet.
// Unlike upsert it does NOT overwrite an existing row, so admin edits
// and deliberate admin deletes survive a backend restart / redeploy.
func (s *productsStore) seedInsert(ctx context.Context, p *Product, hidden bool, sortOrder int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	colors, _ := json.Marshal(stringsOrEmpty(p.Colors))
	sizes, _ := json.Marshal(stringsOrEmpty(p.Sizes))
	tags, _ := json.Marshal(stringsOrEmpty(p.Tags))
	stockBySize, _ := json.Marshal(normalizeStockBySize(p.StockBySize, p.Sizes))
	const q = `INSERT INTO products
(id, name, description, price_cents, pix_price_cents, category, image, back_image,
 colors_json, sizes_json, tags_json, stock, stock_by_size, transparent_image, hidden, sort_order, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO NOTHING`
	_, err := s.db.ExecContext(ctx, rb(q),
		p.ID, p.Name, p.Description, p.PriceCents, p.PixPriceCents, p.Category,
		p.Image, p.BackImage, string(colors), string(sizes), string(tags),
		p.Stock, string(stockBySize), boolToInt(p.TransparentImage), boolToInt(hidden), sortOrder, now, now,
	)
	if err != nil {
		return fmt.Errorf("seed product: %w", err)
	}
	return nil
}

// seedProducts inserts the in-code catalog into the DB on first run only.
// Products are skipped when a row with the same id already exists, so
// admin edits and deliberate deletes survive restarts and redeployments.
// To force a re-seed of a catalog product, delete it from the DB first.
func seedProducts(ctx context.Context, s *productsStore, seed []Product) error {
	for i, p := range seed {
		pp := p
		if err := s.seedInsert(ctx, &pp, false, i); err != nil {
			return err
		}
	}
	return nil
}
