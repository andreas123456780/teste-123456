package main

// Discount coupons — data model, validation, and discount application.
//
// Coupons live in the `coupons` table (see migrations/*/0003_coupons.sql).
// Three kinds are supported:
//
//   - percent        off the item subtotal, 0 < value <= 100
//   - amount         flat cents off the item subtotal, value > 0
//   - free_shipping  zeroes the shipping portion
//
// validateCoupon enforces: active flag, starts_at/expires_at window,
// max_uses budget, and min_subtotal_cents threshold. applyCoupon returns
// the pre-clamped discount so callers can surface it to the user. The
// caller is responsible for persisting the applied discount on the
// order and incrementing used_count inside the checkout transaction.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type couponKind string

const (
	couponPercent      couponKind = "percent"
	couponAmount       couponKind = "amount"
	couponFreeShipping couponKind = "free_shipping"
)

// Coupon is the API/DB shape. Code is always stored and compared in
// uppercase to avoid case-sensitive typos in the UI.
type Coupon struct {
	Code             string     `json:"code"`
	Kind             couponKind `json:"kind"`
	Value            int        `json:"value"`
	MinSubtotalCents int        `json:"minSubtotalCents"`
	MaxUses          int        `json:"maxUses"`
	UsedCount        int        `json:"usedCount"`
	StartsAt         *time.Time `json:"startsAt,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	Active           bool       `json:"active"`
	Note             string     `json:"note,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// DiscountSummary is the caller-visible outcome of applying a coupon.
// All values are non-negative; shipping is the NEW shipping amount (may
// be zero for free_shipping). ItemDiscountCents applies only to the item
// subtotal.
type DiscountSummary struct {
	ItemDiscountCents     int `json:"itemDiscountCents"`
	ShippingDiscountCents int `json:"shippingDiscountCents"`
	TotalDiscountCents    int `json:"totalDiscountCents"`
	NewSubtotalCents      int `json:"newSubtotalCents"`
	NewShippingCents      int `json:"newShippingCents"`
	NewAmountCents        int `json:"newAmountCents"`
}

var couponCodeRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,31}$`)

// normalizeCouponCode trims whitespace and uppercases. Returns "" if the
// input fails the format regex — handlers should treat that as 400.
func normalizeCouponCode(raw string) string {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if !couponCodeRe.MatchString(c) {
		return ""
	}
	return c
}

func validateCouponFields(c *Coupon) error {
	if normalizeCouponCode(c.Code) == "" {
		return errors.New("invalid code (letters, digits, _ or -, 2-32 chars)")
	}
	c.Code = normalizeCouponCode(c.Code)
	switch c.Kind {
	case couponPercent:
		if c.Value <= 0 || c.Value > 100 {
			return errors.New("percent value must be 1-100")
		}
	case couponAmount:
		if c.Value <= 0 || c.Value > 1_000_000 {
			return errors.New("amount value must be 1-1000000 cents")
		}
	case couponFreeShipping:
		c.Value = 0
	default:
		return fmt.Errorf("invalid kind: %q", c.Kind)
	}
	if c.MinSubtotalCents < 0 {
		return errors.New("minSubtotalCents must be >= 0")
	}
	if c.MaxUses < 0 {
		return errors.New("maxUses must be >= 0")
	}
	if c.StartsAt != nil && c.ExpiresAt != nil && c.ExpiresAt.Before(*c.StartsAt) {
		return errors.New("expiresAt must be after startsAt")
	}
	if len(c.Note) > 240 {
		return errors.New("note too long")
	}
	return nil
}

// couponEligibility checks everything that can reject a coupon
// independently of the cart: active flag, time window, max uses.
// Returns a user-safe error message (never leaks internals).
func couponEligibility(c *Coupon, now time.Time) error {
	if !c.Active {
		return errors.New("cupom inativo")
	}
	if c.StartsAt != nil && now.Before(*c.StartsAt) {
		return errors.New("cupom ainda não é válido")
	}
	if c.ExpiresAt != nil && now.After(*c.ExpiresAt) {
		return errors.New("cupom expirado")
	}
	if c.MaxUses > 0 && c.UsedCount >= c.MaxUses {
		return errors.New("cupom esgotado")
	}
	return nil
}

// applyCoupon computes the discount summary for a given cart. It does
// NOT mutate the coupon or the order; persistence is the caller's job.
// Returns a user-safe error if the cart fails the MinSubtotalCents gate.
func applyCoupon(c *Coupon, subtotalCents, shippingCents int) (DiscountSummary, error) {
	if subtotalCents < c.MinSubtotalCents {
		return DiscountSummary{}, fmt.Errorf("subtotal mínimo: %s", moneyBRL(c.MinSubtotalCents))
	}
	out := DiscountSummary{
		NewSubtotalCents: subtotalCents,
		NewShippingCents: shippingCents,
	}
	switch c.Kind {
	case couponPercent:
		d := subtotalCents * c.Value / 100
		if d > subtotalCents {
			d = subtotalCents
		}
		out.ItemDiscountCents = d
		out.NewSubtotalCents = subtotalCents - d
	case couponAmount:
		d := c.Value
		if d > subtotalCents {
			d = subtotalCents
		}
		out.ItemDiscountCents = d
		out.NewSubtotalCents = subtotalCents - d
	case couponFreeShipping:
		out.ShippingDiscountCents = shippingCents
		out.NewShippingCents = 0
	default:
		return DiscountSummary{}, fmt.Errorf("unsupported coupon kind")
	}
	out.TotalDiscountCents = out.ItemDiscountCents + out.ShippingDiscountCents
	out.NewAmountCents = out.NewSubtotalCents + out.NewShippingCents
	return out, nil
}
