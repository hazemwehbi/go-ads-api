-- Bulk imports already processed, keyed by the Idempotency-Key sent by the client.
CREATE TABLE ad_imports (
    user_id         UUID        NOT NULL,
    idempotency_key TEXT        NOT NULL,
    -- Hash of the imported ads, to reject a idempotency_key key reused with another content
    request_hash    TEXT        NOT NULL,
    -- Used to return the list of ads imported by this request, even when request is retried with the same idempotency_key
    ad_ids          BIGINT[]    NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, idempotency_key)
);
