-- 0006_recipient_fields.sql
--
-- Persists the recipient detail fields that SuperFrete requires on the
-- cart payload: CPF/CNPJ plus a split address (number, complement,
-- district, city, state). Older orders are left with NULL on all six
-- columns; the label worker still falls back to ViaCEP for district/
-- city/state when they are empty, so backwards compatibility is
-- preserved.

ALTER TABLE orders ADD COLUMN document TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN address_number TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN address_complement TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN district TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN city TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN state TEXT NOT NULL DEFAULT '';
