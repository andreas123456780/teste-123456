-- 0005_label_retry.sql
--
-- Adds bookkeeping columns so label generation can be retried safely
-- across processes/invocations. The original design assumed a long-
-- running goroutine consumed an in-memory queue; that does not survive
-- on serverless platforms (e.g. Vercel) where the function is killed as
-- soon as the webhook returns 200. We now track every attempt in the
-- DB so a cron-driven worker can pick up paid orders that still don't
-- have a tracking_code and retry them with backoff.

ALTER TABLE orders ADD COLUMN tracking_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN tracking_last_error TEXT;
ALTER TABLE orders ADD COLUMN tracking_attempted_at TIMESTAMP;

-- Speeds up the cron query that scans for paid orders missing a label.
CREATE INDEX IF NOT EXISTS idx_orders_pending_label
    ON orders(status, tracking_code, tracking_attempted_at);
