-- 0004_stock_by_size.sql (Postgres)
--
-- Track stock per size so a sold-out size (e.g. M) doesn't block the
-- rest of the catalog. The column is a JSON map like {"P":8,"M":0}.
-- Legacy products default to '{}' and the storefront treats that as
-- "fall back to the total stock column" for backwards compat.
ALTER TABLE products
  ADD COLUMN IF NOT EXISTS stock_by_size TEXT NOT NULL DEFAULT '{}';
