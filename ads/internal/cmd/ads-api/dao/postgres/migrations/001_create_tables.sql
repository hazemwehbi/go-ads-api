CREATE TABLE ads (
    id          BIGSERIAL   PRIMARY KEY,
    title       TEXT        NOT NULL,
    price_cents BIGINT      NOT NULL,
    photo_url   TEXT        NOT NULL,
    user_id     UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
