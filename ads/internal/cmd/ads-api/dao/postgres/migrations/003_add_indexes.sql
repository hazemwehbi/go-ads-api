-- Initial-install indexes. Use a reviewed concurrent migration on a populated production table.
CREATE INDEX IF NOT EXISTS ads_owner_id_idx ON ads(user_id,id DESC);
CREATE INDEX IF NOT EXISTS ads_price_idx ON ads(price_cents);
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS ads_title_trgm_idx ON ads USING gin(title gin_trgm_ops);
