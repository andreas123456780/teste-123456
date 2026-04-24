-- 0004_stock_by_size.sql (SQLite)
--
-- Track stock per size so a sold-out size (e.g. M) doesn't block the
-- rest of the catalog. SQLite doesn't support IF NOT EXISTS on
-- ADD COLUMN, so the migration is wrapped in applyMigrations' normal
-- once-only semantics.
ALTER TABLE products
  ADD COLUMN stock_by_size TEXT NOT NULL DEFAULT '{}';
