package main

// Admin KPIs endpoint — aggregates orders + order_items to power the
// dashboard at /admin.
//
// All numbers are computed from the orders/order_items tables already
// persisted by /api/checkout and the Stripe webhook (for `paid`
// transitions). We deliberately do NOT maintain separate counter tables
// because the full-table aggregate is cheap at our scale (<<10k rows)
// and avoids a whole class of drift bugs between the counter and the
// source of truth.

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

// adminStats is the JSON shape the frontend consumes. `topProducts` is
// ordered by quantity sold across all paid + shipped orders — i.e. the
// best-selling pieces. `revenueByDay` covers the last 30 days.
type adminStats struct {
	GeneratedAt     time.Time           `json:"generatedAt"`
	Orders          adminOrdersBreakdown `json:"orders"`
	Revenue         adminRevenue         `json:"revenue"`
	TopProducts     []adminTopProduct    `json:"topProducts"`
	RevenueByDay    []adminDailyRevenue  `json:"revenueByDay"`
	RecentOrders    []adminRecentOrder   `json:"recentOrders"`
}

type adminOrdersBreakdown struct {
	Total          int `json:"total"`
	PendingPayment int `json:"pendingPayment"`
	Paid           int `json:"paid"`
	Shipped        int `json:"shipped"`
	Failed         int `json:"failed"`
	Canceled       int `json:"canceled"`
}

type adminRevenue struct {
	GrossCents      int `json:"grossCents"`     // sum of amount_cents of paid + shipped
	ShippingCents   int `json:"shippingCents"`  // sum of shipping_cents ditto
	DiscountCents   int `json:"discountCents"`  // sum of discount_cents ditto
	PaidOrderCount  int `json:"paidOrderCount"` // orders in (paid, shipped)
	AvgTicketCents  int `json:"avgTicketCents"` // grossCents / paidOrderCount
}

type adminTopProduct struct {
	ProductID        string `json:"productId"`
	ProductName      string `json:"productName"`
	Quantity         int    `json:"quantity"`
	GrossCents       int    `json:"grossCents"`
}

type adminDailyRevenue struct {
	Day         string `json:"day"` // YYYY-MM-DD (UTC)
	GrossCents  int    `json:"grossCents"`
	OrderCount  int    `json:"orderCount"`
}

type adminRecentOrder struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	PaymentMethod string    `json:"paymentMethod"`
	AmountCents   int       `json:"amountCents"`
	CustomerName  string    `json:"customerName"`
	CreatedAt     time.Time `json:"createdAt"`
}

// handleAdminStats is the single GET endpoint for the dashboard.
// Returns a 500 if any aggregate query fails — failures are partial by
// nature so we'd rather surface them than lie silently.
func handleAdminStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		stats, err := collectAdminStats(ctx, db)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stats unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, stats)
	}
}

// collectAdminStats runs the 5 aggregations sequentially. Sequential
// (not concurrent) keeps the code simple and is fine at our scale — the
// queries individually return in single-digit ms.
func collectAdminStats(ctx context.Context, db *sql.DB) (*adminStats, error) {
	breakdown, err := queryOrdersBreakdown(ctx, db)
	if err != nil {
		return nil, err
	}
	revenue, err := queryRevenue(ctx, db)
	if err != nil {
		return nil, err
	}
	top, err := queryTopProducts(ctx, db, 10)
	if err != nil {
		return nil, err
	}
	daily, err := queryRevenueByDay(ctx, db, 30)
	if err != nil {
		return nil, err
	}
	recent, err := queryRecentOrders(ctx, db, 10)
	if err != nil {
		return nil, err
	}
	return &adminStats{
		GeneratedAt:  time.Now().UTC(),
		Orders:       breakdown,
		Revenue:      revenue,
		TopProducts:  top,
		RevenueByDay: daily,
		RecentOrders: recent,
	}, nil
}

func queryOrdersBreakdown(ctx context.Context, db *sql.DB) (adminOrdersBreakdown, error) {
	rows, err := db.QueryContext(ctx, rb(`SELECT status, COUNT(*) FROM orders GROUP BY status`))
	if err != nil {
		return adminOrdersBreakdown{}, err
	}
	defer rows.Close()
	var b adminOrdersBreakdown
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return adminOrdersBreakdown{}, err
		}
		b.Total += n
		switch status {
		case "pending_payment":
			b.PendingPayment = n
		case "paid":
			b.Paid = n
		case "shipped":
			b.Shipped = n
		case "failed":
			b.Failed = n
		case "canceled":
			b.Canceled = n
		}
	}
	return b, rows.Err()
}

func queryRevenue(ctx context.Context, db *sql.DB) (adminRevenue, error) {
	// Count everything that actually translated into money: paid and
	// shipped both represent captured payments. pending_payment does
	// not — those are reservations.
	var r adminRevenue
	row := db.QueryRowContext(ctx, rb(`
		SELECT
			COALESCE(SUM(amount_cents), 0),
			COALESCE(SUM(shipping_cents), 0),
			COALESCE(SUM(discount_cents), 0),
			COUNT(*)
		FROM orders
		WHERE status IN ('paid', 'shipped')`))
	if err := row.Scan(&r.GrossCents, &r.ShippingCents, &r.DiscountCents, &r.PaidOrderCount); err != nil {
		return adminRevenue{}, err
	}
	if r.PaidOrderCount > 0 {
		r.AvgTicketCents = r.GrossCents / r.PaidOrderCount
	}
	return r, nil
}

func queryTopProducts(ctx context.Context, db *sql.DB, limit int) ([]adminTopProduct, error) {
	// Join order_items with orders to restrict to paid+shipped orders
	// (we don't care about abandoned reservations).
	rows, err := db.QueryContext(ctx, rb(`
		SELECT i.product_id, i.product_name,
			SUM(i.quantity), SUM(i.quantity * i.unit_price_cents)
		FROM order_items i
		JOIN orders o ON o.id = i.order_id
		WHERE o.status IN ('paid', 'shipped')
		GROUP BY i.product_id, i.product_name
		ORDER BY SUM(i.quantity) DESC, i.product_id ASC
		LIMIT ?`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]adminTopProduct, 0, limit)
	for rows.Next() {
		var p adminTopProduct
		if err := rows.Scan(&p.ProductID, &p.ProductName, &p.Quantity, &p.GrossCents); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func queryRevenueByDay(ctx context.Context, db *sql.DB, days int) ([]adminDailyRevenue, error) {
	// Both dialects serialize timestamps as RFC3339-ish strings that
	// begin with YYYY-MM-DD. substr(…, 1, 10) is cheaper and more
	// portable than DATE() — the modernc sqlite driver emits a 'T'
	// separator that DATE() chokes on.
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	rows, err := db.QueryContext(ctx, rb(`
		SELECT substr(CAST(created_at AS TEXT), 1, 10) AS day,
			COALESCE(SUM(amount_cents), 0), COUNT(*)
		FROM orders
		WHERE status IN ('paid', 'shipped')
			AND created_at >= ?
		GROUP BY day
		ORDER BY day ASC`), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]adminDailyRevenue, 0, days)
	for rows.Next() {
		var d adminDailyRevenue
		if err := rows.Scan(&d.Day, &d.GrossCents, &d.OrderCount); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func queryRecentOrders(ctx context.Context, db *sql.DB, limit int) ([]adminRecentOrder, error) {
	rows, err := db.QueryContext(ctx, rb(`
		SELECT id, status, payment_method, amount_cents, name, created_at
		FROM orders
		ORDER BY created_at DESC
		LIMIT ?`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]adminRecentOrder, 0, limit)
	for rows.Next() {
		var o adminRecentOrder
		if err := rows.Scan(&o.ID, &o.Status, &o.PaymentMethod, &o.AmountCents, &o.CustomerName, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
