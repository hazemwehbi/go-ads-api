-- Publication history deliberately survives ad deletion; imports never write it.
CREATE TABLE ad_publications (
 ad_id BIGINT PRIMARY KEY,
 owner_id UUID NOT NULL,
 published_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX ad_publications_owner_time_idx ON ad_publications(owner_id,published_at);
ALTER TABLE ads ADD CONSTRAINT ads_price_valid CHECK(price_cents BETWEEN 0 AND 1000000000000);
ALTER TABLE ads ADD CONSTRAINT ads_title_valid CHECK(length(btrim(title)) BETWEEN 1 AND 200);
ALTER TABLE ads ADD CONSTRAINT ads_photo_length CHECK(length(photo_url) BETWEEN 1 AND 2048);
