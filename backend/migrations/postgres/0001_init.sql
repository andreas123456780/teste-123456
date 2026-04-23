-- 0001_init.sql (Postgres)
--
-- Mirrors the SQLite schema in migrations/sqlite/0001_init.sql with
-- Postgres-native types: BIGSERIAL for the auto-increment column and
-- TIMESTAMP WITH TIME ZONE for temporal fields. Everything else —
-- foreign keys, indexes, ON DELETE CASCADE — is syntactically
-- compatible between the two engines so both migrations stay aligned.

CREATE TABLE IF NOT EXISTS orders (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL,
    email                 TEXT NOT NULL,
    address               TEXT NOT NULL,
    zip                   TEXT NOT NULL,
    payment_method        TEXT NOT NULL,
    status                TEXT NOT NULL,
    total_cents           INTEGER NOT NULL,
    shipping_cents        INTEGER NOT NULL DEFAULT 0,
    amount_cents          INTEGER NOT NULL,
    shipping_service_id   INTEGER,
    shipping_service_name TEXT,
    payment_intent_id     TEXT,
    tracking_code         TEXT,
    tracking_url          TEXT,
    label_url             TEXT,
    superfrete_order_id   TEXT,
    created_at            TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_orders_email  ON orders(email);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_pi     ON orders(payment_intent_id);

CREATE TABLE IF NOT EXISTS order_items (
    order_id         TEXT    NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    line_no          INTEGER NOT NULL,
    product_id       TEXT    NOT NULL,
    product_name     TEXT    NOT NULL,
    size             TEXT,
    color            TEXT,
    quantity         INTEGER NOT NULL,
    unit_price_cents INTEGER NOT NULL,
    PRIMARY KEY (order_id, line_no)
);

CREATE TABLE IF NOT EXISTS payment_events (
    id                BIGSERIAL PRIMARY KEY,
    order_id          TEXT REFERENCES orders(id) ON DELETE CASCADE,
    event_type        TEXT NOT NULL,
    payment_intent_id TEXT,
    raw_payload       TEXT,
    created_at        TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_order ON payment_events(order_id);
CREATE INDEX IF NOT EXISTS idx_events_pi    ON payment_events(payment_intent_id);
