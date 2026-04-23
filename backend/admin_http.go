package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// adminAuth is a thin legacy-token-only wrapper around adminAuthFromCfg.
// Retained so existing tests and the minimal legacy-token-only
// configuration keep working without duplicating the header logic.
func adminAuth(expected string, next http.HandlerFunc) http.HandlerFunc {
	return adminAuthFromCfg(adminAuthCfg{legacyToken: expected}, next)
}

// suppress unused-import warning when this file is compiled alone.
var _ = subtle.ConstantTimeCompare

// adminProductPayload is the shape accepted by the admin CRUD endpoints.
// `hidden` and `sortOrder` are optional overrides that let the operator
// pre-release an item (hidden=true) or reorder the grid (sortOrder).
type adminProductPayload struct {
	Product
	Hidden    *bool `json:"hidden,omitempty"`
	SortOrder *int  `json:"sortOrder,omitempty"`
}

// handleAdminProducts lists all products (hidden included) and creates
// new ones.
func handleAdminProducts(store *productsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		switch r.Method {
		case http.MethodGet:
			list, err := store.listAdmin(ctx)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
				return
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var p adminProductPayload
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			if err := validateProduct(&p.Product); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			hidden := false
			if p.Hidden != nil {
				hidden = *p.Hidden
			}
			sortOrder := 1_000 // new products go to the end by default
			if p.SortOrder != nil {
				sortOrder = *p.SortOrder
			}
			if err := store.upsert(ctx, &p.Product, hidden, sortOrder); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save failed"})
				return
			}
			writeJSON(w, http.StatusCreated, p.Product)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

// handleAdminProductByID handles PUT/DELETE for a specific product and a
// GET that includes hidden rows (useful for the admin detail screen).
func handleAdminProductByID(store *productsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/admin/products/")
		if id == "" || strings.Contains(id, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		switch r.Method {
		case http.MethodGet:
			p, err := store.get(ctx, id)
			if errors.Is(err, errProductNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "product not found"})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
				return
			}
			writeJSON(w, http.StatusOK, p)
		case http.MethodPut:
			var p adminProductPayload
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			// URL id wins over body id — safer.
			p.Product.ID = id
			if err := validateProduct(&p.Product); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			hidden := false
			if p.Hidden != nil {
				hidden = *p.Hidden
			}
			sortOrder := 0
			if p.SortOrder != nil {
				sortOrder = *p.SortOrder
			}
			if err := store.upsert(ctx, &p.Product, hidden, sortOrder); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save failed"})
				return
			}
			writeJSON(w, http.StatusOK, p.Product)
		case http.MethodDelete:
			if err := store.delete(ctx, id); err != nil {
				if errors.Is(err, errProductNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "product not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

func validateProduct(p *Product) error {
	if _, ok := safeString(p.ID, 60); !ok {
		return errors.New("invalid id")
	}
	if _, ok := safeString(p.Name, 120); !ok {
		return errors.New("invalid name")
	}
	if p.PriceCents <= 0 || p.PriceCents > 10_000_000 {
		return errors.New("invalid priceCents")
	}
	if p.PixPriceCents <= 0 || p.PixPriceCents > p.PriceCents {
		return errors.New("invalid pixPriceCents")
	}
	if p.Stock < 0 || p.Stock > 10_000 {
		return errors.New("invalid stock")
	}
	return nil
}
