-- 0006_recipient_fields.sql (Postgres)
--
-- Persists the recipient detail fields that SuperFrete requires on the
-- cart payload: CPF/CNPJ plus a split address (number, complement,
-- district, city, state). Older orders are left with '' on all six
-- columns; the label worker still falls back to ViaCEP for district/
-- city/state when they are empty, so backwards compatibility is
-- preserved.

ALTER TABLE orders ADD COLUMN IF NOT EXISTS document TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS address_number TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS address_complement TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS district TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS city TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT '';
