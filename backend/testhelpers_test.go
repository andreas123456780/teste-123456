package main

// Shared test plumbing.
//
// newTestStore opens an anonymous shared-memory SQLite, runs migrations
// and returns an orderStore wired to it. Callers are expected to defer
// the returned cleanup func.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) (*orderStore, *sql.DB, func()) {
	t.Helper()
	// `cache=shared` with file::memory: keeps all handles in the pool
	// talking to the same in-memory database. Without it each conn sees
	// its own empty schema and migrations must re-run — that's fine
	// functionally but surprising during test debugging.
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1) // serialize to dodge memory-sharing edge cases
	currentDialect = dialectSQLite
	if err := applyMigrations(db, dialectSQLite); err != nil {
		_ = db.Close()
		t.Fatalf("migrate: %v", err)
	}
	store := newOrderStore(db)
	return store, db, func() { _ = db.Close() }
}

// newTestProducts seeds an in-memory product store with the static
// `catalog` so handler tests that read `/api/products` get realistic
// data without coupling to the migration fixtures.
func newTestProducts(t *testing.T, db *sql.DB) *productsStore {
	t.Helper()
	store := newProductsStore(db)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := seedProducts(ctx, store, catalog); err != nil {
		t.Fatalf("seed products: %v", err)
	}
	return store
}

// newTestCoupons returns an empty coupons store backed by the shared
// test DB. Tests that exercise the coupon flow populate it explicitly.
func newTestCoupons(t *testing.T, db *sql.DB) *couponsStore {
	t.Helper()
	return newCouponsStore(db)
}

// validCheckoutDefaults returns the recipient fields required by the
// current checkout validation (full name, CPF, split address). Tests
// spread this into their CheckoutRequest to avoid repeating the
// boilerplate after every schema change.
//
// The CPF below is a placeholder that passes the digits-only + not-all-
// repeated check; it is never sent to SuperFrete during tests.
func validCheckoutDefaults() CheckoutRequest {
	return CheckoutRequest{
		Name:              "Andreas Teste",
		Email:             "andreas@example.com",
		Document:          "12345678909",
		Address:           "Rua das Flores",
		AddressNumber:     "123",
		AddressComplement: "",
		District:          "Centro",
		City:              "São Paulo",
		State:             "SP",
		ZipCode:           "01000-000",
	}
}

// putTestOrder is a small helper used in several tests to create a
// minimal pending order.
func putTestOrder(t *testing.T, store *orderStore, id, email, method string, amount, shipping int) *pendingOrder {
	t.Helper()
	o := &pendingOrder{
		ID:            id,
		Name:          "Andreas Teste",
		Email:         email,
		Address:       "Rua X, 1",
		Zip:           "01000-000",
		PaymentMethod: method,
		Status:        "pending_payment",
		TotalCents:    amount - shipping,
		ShippingCents: shipping,
		AmountCents:   amount,
		CreatedAt:     time.Now().UTC(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := store.create(ctx, o); err != nil {
		t.Fatalf("create test order: %v", err)
	}
	return o
}
