-- Products table backs the public catalog as well as the admin CRUD
-- endpoints. The SPA seed in frontend/src/data/fallback.ts remains the
-- offline fallback; the authoritative list lives here once the
-- application has booted at least once.
CREATE TABLE IF NOT EXISTS products (
    id               TEXT    PRIMARY KEY,
    name             TEXT    NOT NULL,
    description      TEXT    NOT NULL DEFAULT '',
    price_cents      INTEGER NOT NULL,
    pix_price_cents  INTEGER NOT NULL,
    category         TEXT    NOT NULL DEFAULT '',
    image            TEXT    NOT NULL DEFAULT '',
    back_image       TEXT    NOT NULL DEFAULT '',
    colors_json      TEXT    NOT NULL DEFAULT '[]',
    sizes_json       TEXT    NOT NULL DEFAULT '[]',
    tags_json        TEXT    NOT NULL DEFAULT '[]',
    stock            INTEGER NOT NULL DEFAULT 0,
    -- Hidden products stay in the DB (for historical order references)
    -- but don't appear in the public catalog.
    hidden           INTEGER NOT NULL DEFAULT 0,
    sort_order       INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT    NOT NULL,
    updated_at       TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_products_category ON products(category);
CREATE INDEX IF NOT EXISTS idx_products_sort ON products(sort_order);
