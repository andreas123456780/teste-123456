-- 0008_order_user_link.sql
--
-- Links an order to the userAccount that placed it. NULL for legacy
-- orders; in that case the account endpoint falls back to matching by
-- order.email == users.email_lower so customers see their history
-- even for purchases made before the login system existed.

ALTER TABLE orders ADD COLUMN IF NOT EXISTS user_id TEXT;
CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id);
