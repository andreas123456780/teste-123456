-- 0005_label_retry.sql (Postgres)
--
-- Mirrors migrations/sqlite/0005_label_retry.sql. See that file for
-- the rationale.

ALTER TABLE orders ADD COLUMN IF NOT EXISTS tracking_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS tracking_last_error TEXT;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS tracking_attempted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_orders_pending_label
    ON orders(status, tracking_code, tracking_attempted_at);
