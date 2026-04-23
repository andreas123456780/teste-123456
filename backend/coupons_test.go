package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newActiveCoupon(c *Coupon) *Coupon {
	if c.Kind == "" {
		c.Kind = couponPercent
	}
	if c.Code == "" {
		c.Code = "NAST10"
	}
	c.Active = true
	c.CreatedAt = time.Now().UTC().Add(-time.Hour)
	return c
}

func TestApplyCoupon_Percent(t *testing.T) {
	c := newActiveCoupon(&Coupon{Kind: couponPercent, Value: 10})
	sum, err := applyCoupon(c, 10_000, 1_500)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if sum.ItemDiscountCents != 1_000 || sum.NewSubtotalCents != 9_000 {
		t.Fatalf("bad percent discount: %+v", sum)
	}
	if sum.NewShippingCents != 1_500 {
		t.Fatalf("shipping should be untouched: %+v", sum)
	}
	if sum.NewAmountCents != 10_500 {
		t.Fatalf("amount mismatch: %+v", sum)
	}
}

func TestApplyCoupon_AmountClampsToSubtotal(t *testing.T) {
	c := newActiveCoupon(&Coupon{Kind: couponAmount, Value: 15_000})
	sum, _ := applyCoupon(c, 8_000, 0)
	if sum.ItemDiscountCents != 8_000 || sum.NewSubtotalCents != 0 {
		t.Fatalf("amount should clamp to subtotal: %+v", sum)
	}
}

func TestApplyCoupon_FreeShipping(t *testing.T) {
	c := newActiveCoupon(&Coupon{Kind: couponFreeShipping})
	sum, _ := applyCoupon(c, 10_000, 2_000)
	if sum.ShippingDiscountCents != 2_000 || sum.NewShippingCents != 0 {
		t.Fatalf("shipping not zeroed: %+v", sum)
	}
	if sum.NewSubtotalCents != 10_000 {
		t.Fatalf("subtotal altered: %+v", sum)
	}
}

func TestApplyCoupon_RejectsBelowMinSubtotal(t *testing.T) {
	c := newActiveCoupon(&Coupon{Kind: couponPercent, Value: 10, MinSubtotalCents: 15_000})
	if _, err := applyCoupon(c, 10_000, 0); err == nil {
		t.Fatal("expected error for below-min subtotal")
	}
}

func TestCouponEligibility_ExpiredRejected(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	c := newActiveCoupon(&Coupon{Kind: couponPercent, Value: 10, ExpiresAt: &past})
	if err := couponEligibility(c, time.Now().UTC()); err == nil {
		t.Fatal("expected expired error")
	}
}

func TestCouponEligibility_MaxUsesReached(t *testing.T) {
	c := newActiveCoupon(&Coupon{Kind: couponPercent, Value: 10, MaxUses: 3, UsedCount: 3})
	if err := couponEligibility(c, time.Now().UTC()); err == nil {
		t.Fatal("expected exhausted error")
	}
}

func TestNormalizeCouponCode(t *testing.T) {
	cases := map[string]string{
		"  nast10 ": "NAST10",
		"FREE_BR":   "FREE_BR",
		"a":         "", // too short
		"bad code":  "", // whitespace inside
		"NAST-2026": "NAST-2026",
	}
	for in, want := range cases {
		if got := normalizeCouponCode(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCouponsStore_CRUD(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	store := newCouponsStore(db)
	ctx := context.Background()
	c := Coupon{Code: "NAST10", Kind: couponPercent, Value: 10, Active: true}
	if err := validateCouponFields(&c); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := store.upsert(ctx, &c); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := store.get(ctx, "NAST10")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Value != 10 || !got.Active {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	// update value
	c.Value = 15
	if err := store.upsert(ctx, &c); err != nil {
		t.Fatalf("reupsert: %v", err)
	}
	got, _ = store.get(ctx, "NAST10")
	if got.Value != 15 {
		t.Fatalf("update did not take: %+v", got)
	}
	list, err := store.list(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", err, list)
	}
	if err := store.delete(ctx, "NAST10"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestHandleCouponValidate_HappyPath(t *testing.T) {
	_, db, cleanup := newTestStore(t)
	defer cleanup()
	store := newCouponsStore(db)
	c := Coupon{Code: "NAST10", Kind: couponPercent, Value: 10, Active: true}
	_ = validateCouponFields(&c)
	_ = store.upsert(context.Background(), &c)

	body := `{"code":"nast10","subtotalCents":10000,"shippingCents":1500}`
	req := httptest.NewRequest(http.MethodPost, "/api/coupons/validate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	handleCouponValidate(store)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var out validateCouponResponse
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Discount.ItemDiscountCents != 1000 {
		t.Fatalf("unexpected discount: %+v", out.Discount)
	}
	if out.Coupon.Code != "NAST10" {
		t.Fatalf("unexpected code: %+v", out.Coupon)
	}
}

func TestCheckout_WithCoupon_AppliesAndIncrements(t *testing.T) {
	orders, db, cleanup := newTestStore(t)
	defer cleanup()
	prods := newTestProducts(t, db)
	coupons := newCouponsStore(db)
	c := Coupon{Code: "NAST10", Kind: couponPercent, Value: 10, Active: true}
	_ = validateCouponFields(&c)
	_ = coupons.upsert(context.Background(), &c)

	body := CheckoutRequest{
		Items:         []CartItem{{ProductID: "p-tee-bw-black", Quantity: 1, Size: "M", Color: "preto"}},
		Name:          "Andreas",
		Email:         "a@example.com",
		Address:       "Rua X, 1",
		ZipCode:       "01000-000",
		PaymentMethod: "card",
		CouponCode:    "nast10",
	}
	b, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(b))
	handleCheckout(orders, prods, coupons, []byte("k"))(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var out CheckoutResponse
	_ = json.NewDecoder(rr.Body).Decode(&out)
	if out.CouponCode != "NAST10" || out.DiscountCents != 899 {
		t.Fatalf("expected 10%% off 8990 = 899, got %+v", out)
	}
	if out.TotalCents != 8990-899 {
		t.Fatalf("total wrong: %+v", out)
	}
	// used_count incremented?
	got, _ := coupons.get(context.Background(), "NAST10")
	if got.UsedCount != 1 {
		t.Fatalf("used count not incremented: %+v", got)
	}
}

func TestCheckout_WithExpiredCoupon_Rejects(t *testing.T) {
	orders, db, cleanup := newTestStore(t)
	defer cleanup()
	prods := newTestProducts(t, db)
	coupons := newCouponsStore(db)
	past := time.Now().UTC().Add(-time.Hour)
	c := Coupon{Code: "OLDIE", Kind: couponPercent, Value: 10, Active: true, ExpiresAt: &past}
	_ = validateCouponFields(&c)
	_ = coupons.upsert(context.Background(), &c)

	body := CheckoutRequest{
		Items:         []CartItem{{ProductID: "p-tee-bw-black", Quantity: 1, Size: "M", Color: "preto"}},
		Name:          "Andreas",
		Email:         "a@example.com",
		Address:       "Rua X",
		ZipCode:       "01000-000",
		PaymentMethod: "card",
		CouponCode:    "OLDIE",
	}
	b, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(b))
	handleCheckout(orders, prods, coupons, []byte("k"))(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
