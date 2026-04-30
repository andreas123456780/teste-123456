-- 0009_settings.sql
--
-- Generic key/value site_settings table. Used by the storefront to
-- toggle non-product UI features at runtime (e.g. the next-drop
-- countdown). Keys are slugged identifiers; values are stored as
-- text and parsed by the caller (JSON for structured payloads).

CREATE TABLE IF NOT EXISTS site_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
