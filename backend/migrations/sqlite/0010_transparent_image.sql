-- 0010_transparent_image.sql (SQLite)
--
-- Per-product flag that marks the product image as a transparent PNG.
-- When set, the storefront drops the white photo backdrop on the card
-- and modal so the cutout floats on the page background. Defaults to
-- 0 (legacy white-backdrop behavior) so existing rows are untouched.
ALTER TABLE products
  ADD COLUMN transparent_image INTEGER NOT NULL DEFAULT 0;
