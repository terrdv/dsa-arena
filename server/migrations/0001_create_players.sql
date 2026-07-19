CREATE TABLE players (
    id BIGSERIAL PRIMARY KEY,
    username text,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
