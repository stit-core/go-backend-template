-- 0004_outbox: общий outbox для доменных событий.
-- Используется outboxkit (golibinfra). Транзакционная гарантия:
-- доменное изменение и публикация события — в одной транзакции.
-- Воркер (cmd/worker) сканирует unpublished по partial index и шлёт в NATS.

CREATE TABLE outbox (
    id           BIGSERIAL    PRIMARY KEY,
    subject      TEXT         NOT NULL,
    payload      JSONB        NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL;
