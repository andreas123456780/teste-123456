package main

// HTTP handlers for coupons.
//
//   /api/coupons/validate       POST — public, previews discount for a cart
//   /api/admin/coupons          GET/POST — admin CRUD list/create
//   /api/admin/coupons/:code    GET/PUT/DELETE — admin CRUD single

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// validateCouponRequest is the payload the cart sends before checkout.
// subtotalCents/shippingCents are the browser-computed amounts; we
// re-validate the discount server-side at checkout, so this is purely a
// preview endpoint.
type validateCouponRequest struct {
	Code           string `json:"code"`
	SubtotalCents  int    `json:"subtotalCents"`
	ShippingCents  int    `json:"shippingCents"`
}

type validateCouponResponse struct {
	Coupon   publicCoupon    `json:"coupon"`
	Discount DiscountSummary `json:"discount"`
}

// publicCoupon is a subset of Coupon safe to return to anonymous
// visitors: we hide used_count / note so an attacker can't enumerate
// internal notes.
type publicCoupon struct {
	Code             string     `json:"code"`
	Kind             couponKind `json:"kind"`
	Value            int        `json:"value"`
	MinSubtotalCents int        `json:"minSubtotalCents"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}

func toPublicCoupon(c *Coupon) publicCoupon {
	return publicCoupon{
		Code:             c.Code,
		Kind:             c.Kind,
		Value:            c.Value,
		MinSubtotalCents: c.MinSubtotalCents,
		ExpiresAt:        c.ExpiresAt,
	}
}

func handleCouponValidate(store *couponsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req validateCouponRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		code := normalizeCouponCode(req.Code)
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "código inválido"})
			return
		}
		if req.SubtotalCents < 0 || req.SubtotalCents > 50_000_000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subtotal inválido"})
			return
		}
		if req.ShippingCents < 0 || req.ShippingCents > 500_000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "frete inválido"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		c, err := store.get(ctx, code)
		if errors.Is(err, errCouponNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "cupom não encontrado"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
			return
		}
		if err := couponEligibility(c, time.Now().UTC()); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		summary, err := applyCoupon(c, req.SubtotalCents, req.ShippingCents)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, validateCouponResponse{
			Coupon:   toPublicCoupon(c),
			Discount: summary,
		})
	}
}

// ----- Admin -----

type adminCouponPayload struct {
	Code             string     `json:"code"`
	Kind             couponKind `json:"kind"`
	Value            int        `json:"value"`
	MinSubtotalCents int        `json:"minSubtotalCents"`
	MaxUses          int        `json:"maxUses"`
	StartsAt         *time.Time `json:"startsAt,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	Active           *bool      `json:"active,omitempty"`
	Note             string     `json:"note,omitempty"`
}

func (p adminCouponPayload) toCoupon() Coupon {
	active := true
	if p.Active != nil {
		active = *p.Active
	}
	return Coupon{
		Code:             p.Code,
		Kind:             p.Kind,
		Value:            p.Value,
		MinSubtotalCents: p.MinSubtotalCents,
		MaxUses:          p.MaxUses,
		StartsAt:         p.StartsAt,
		ExpiresAt:        p.ExpiresAt,
		Active:           active,
		Note:             strings.TrimSpace(p.Note),
	}
}

func handleAdminCoupons(store *couponsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		switch r.Method {
		case http.MethodGet:
			list, err := store.list(ctx)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
				return
			}
			if list == nil {
				list = []Coupon{}
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var p adminCouponPayload
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			c := p.toCoupon()
			if err := validateCouponFields(&c); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if err := store.upsert(ctx, &c); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save failed"})
				return
			}
			writeJSON(w, http.StatusCreated, c)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

func handleAdminCouponByCode(store *couponsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.URL.Path, "/api/admin/coupons/")
		code := normalizeCouponCode(raw)
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid code"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		switch r.Method {
		case http.MethodGet:
			c, err := store.get(ctx, code)
			if errors.Is(err, errCouponNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
				return
			}
			writeJSON(w, http.StatusOK, c)
		case http.MethodPut:
			var p adminCouponPayload
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			// URL code wins over body so renames go through
			// delete-then-create.
			p.Code = code
			c := p.toCoupon()
			if err := validateCouponFields(&c); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			// Preserve the historical counter.
			existing, err := store.get(ctx, code)
			if err == nil {
				c.UsedCount = existing.UsedCount
				c.CreatedAt = existing.CreatedAt
			}
			if err := store.upsert(ctx, &c); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save failed"})
				return
			}
			writeJSON(w, http.StatusOK, c)
		case http.MethodDelete:
			if err := store.delete(ctx, code); err != nil {
				if errors.Is(err, errCouponNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
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
