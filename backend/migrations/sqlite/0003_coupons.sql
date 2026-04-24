-- Coupons table. Admin creates/edits via /api/admin/coupons; the public
-- /api/coupons/validate endpoint is read-only and used by the cart to
-- preview the discount before checkout.
--
-- kind values:
--   'percent'        -> off the item subtotal (value 0-100)
--   'amount'         -> flat BRL cents off the item subtotal
--   'free_shipping'  -> zero out the shipping portion of the order
--
-- `value` is reused across kinds: percent keeps 0-100, amount keeps cents
-- and is ignored for free_shipping. `min_subtotal_cents` gates eligibility
-- by cart size; `max_uses`=0 means unlimited. `used_count` is incremented
-- inside the checkout tx to keep the counter consistent with actual
-- orders.
CREATE TABLE IF NOT EXISTS coupons (
    code                TEXT    PRIMARY KEY,
    kind                TEXT    NOT NULL,
    value               INTEGER NOT NULL DEFAULT 0,
    min_subtotal_cents  INTEGER NOT NULL DEFAULT 0,
    max_uses            INTEGER NOT NULL DEFAULT 0,
    used_count          INTEGER NOT NULL DEFAULT 0,
    starts_at           TIMESTAMP,
    expires_at          TIMESTAMP,
    active              INTEGER NOT NULL DEFAULT 1,
    note                TEXT    NOT NULL DEFAULT '',
    created_at          TIMESTAMP NOT NULL,
    updated_at          TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons(active);

-- Orders gain coupon_code + discount_cents so the admin UI can surface
-- revenue net of promotions and so the checkout receipt reflects the
-- price the customer actually paid.
ALTER TABLE orders ADD COLUMN coupon_code    TEXT;
ALTER TABLE orders ADD COLUMN discount_cents INTEGER NOT NULL DEFAULT 0;
