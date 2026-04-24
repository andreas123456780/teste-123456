-- See sqlite/0003_coupons.sql for the authoritative comment. Schema is
-- the same, with Postgres-native timestamp and boolean types.
CREATE TABLE IF NOT EXISTS coupons (
    code                TEXT        PRIMARY KEY,
    kind                TEXT        NOT NULL,
    value               INTEGER     NOT NULL DEFAULT 0,
    min_subtotal_cents  INTEGER     NOT NULL DEFAULT 0,
    max_uses            INTEGER     NOT NULL DEFAULT 0,
    used_count          INTEGER     NOT NULL DEFAULT 0,
    starts_at           TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ,
    active              BOOLEAN     NOT NULL DEFAULT TRUE,
    note                TEXT        NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons(active);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS coupon_code    TEXT;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS discount_cents INTEGER NOT NULL DEFAULT 0;
