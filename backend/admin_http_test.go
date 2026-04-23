package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminProducts_RequiresToken(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	prods := newTestProducts(t, db)
	h := adminAuth("s3cret", handleAdminProducts(prods))

	// no token → 401
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/admin/products", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no token → 401, got %d", rr.Code)
	}

	// wrong token → 401
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)
	req.Header.Set("X-Admin-Token", "wrong")
	h(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token → 401, got %d", rr.Code)
	}
}

func TestAdminProducts_Unconfigured503(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	prods := newTestProducts(t, db)
	h := adminAuth("", handleAdminProducts(prods))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)
	req.Header.Set("X-Admin-Token", "whatever")
	h(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured → 503, got %d", rr.Code)
	}
}

func TestAdminProducts_CreateAndList(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	prods := newTestProducts(t, db)
	h := adminAuth("t0k3n", handleAdminProducts(prods))

	payload := adminProductPayload{
		Product: Product{
			ID:            "p-test-hoodie",
			Name:          "NAST Hoodie Test",
			Description:   "x",
			PriceCents:    25000,
			PixPriceCents: 22500,
			Category:      "hoodies",
			Image:         "/img.png",
			Colors:        []string{"preto"},
			Sizes:         []string{"M", "G"},
			Tags:          []string{"novo"},
			Stock:         10,
		},
	}
	b, _ := json.Marshal(payload)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products", bytes.NewReader(b))
	req.Header.Set("X-Admin-Token", "t0k3n")
	h(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create → 201, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)
	req.Header.Set("X-Admin-Token", "t0k3n")
	h(rr, req)
	var out []Product
	_ = json.NewDecoder(rr.Body).Decode(&out)
	found := false
	for _, p := range out {
		if p.ID == "p-test-hoodie" {
			found = true
			if p.Stock != 10 {
				t.Fatalf("stock roundtrip failed: %d", p.Stock)
			}
		}
	}
	if !found {
		t.Fatalf("created product missing from list")
	}
}

func TestAdminProduct_UpdateAndDelete(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	prods := newTestProducts(t, db)
	list := adminAuth("k", handleAdminProducts(prods))
	one := adminAuth("k", handleAdminProductByID(prods))

	// seed via create
	payload := adminProductPayload{
		Product: Product{
			ID: "p-item", Name: "Original", PriceCents: 9900, PixPriceCents: 8910, Stock: 5,
		},
	}
	b, _ := json.Marshal(payload)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products", bytes.NewReader(b))
	req.Header.Set("X-Admin-Token", "k")
	list(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}

	// update name
	payload.Product.Name = "Updated"
	payload.Product.Stock = 99
	b, _ = json.Marshal(payload)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/admin/products/p-item", bytes.NewReader(b))
	req.Header.Set("X-Admin-Token", "k")
	one(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rr.Code, rr.Body.String())
	}

	// delete
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/products/p-item", nil)
	req.Header.Set("X-Admin-Token", "k")
	one(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rr.Code)
	}

	// gone
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/products/p-item", nil)
	req.Header.Set("X-Admin-Token", "k")
	one(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get deleted: %d", rr.Code)
	}
}

func TestDecrementStock(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	prods := newTestProducts(t, db)

	rem, err := prods.decrementStock(context.Background(), "p-tee-bw-black", 1)
	if err != nil {
		t.Fatalf("decrement: %v", err)
	}
	if rem != 0 {
		t.Fatalf("expected fully decremented, got remainder %d", rem)
	}

	// oversell: more than stock
	before, _ := prods.get(context.Background(), "p-tee-bw-black")
	huge := before.Stock + 100
	rem, err = prods.decrementStock(context.Background(), "p-tee-bw-black", huge)
	if err != nil {
		t.Fatalf("decrement huge: %v", err)
	}
	if rem == 0 {
		t.Fatalf("expected remainder > 0 on oversell")
	}
	after, _ := prods.get(context.Background(), "p-tee-bw-black")
	if after.Stock != 0 {
		t.Fatalf("stock should clamp to 0, got %d", after.Stock)
	}
}
