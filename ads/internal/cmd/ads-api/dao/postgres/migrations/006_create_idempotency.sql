-- Preserve existing bulk receipts and isolate single-create keys.
BEGIN;
ALTER TABLE ad_imports ADD COLUMN operation TEXT NOT NULL DEFAULT 'import'
    CHECK (operation IN ('import', 'create'));
ALTER TABLE ad_imports DROP CONSTRAINT ad_imports_pkey;
ALTER TABLE ad_imports ADD PRIMARY KEY (user_id, operation, idempotency_key);
COMMIT;
