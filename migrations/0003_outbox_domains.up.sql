-- 0003_outbox_domains: per-domain transactional-outbox таблицы (outboxkit multi-instance,
-- pgstore.NewStoreFor). Структура зеркалит embedded outboxkit/pgstore DDL; партиции на месяц
-- держит partition-keeper job каждого инстанса. Default-таблица `outbox` (embedded-миграция
-- outboxkit) этим сервисом не используется.
DO $$
DECLARE
    tbl    TEXT;
    base   DATE := DATE_TRUNC('month', NOW())::DATE;
    i      INT;
    pstart DATE;
    pend   DATE;
    pname  TEXT;
BEGIN
    FOREACH tbl IN ARRAY ARRAY['outbox_user', 'outbox_verification'] LOOP
        EXECUTE FORMAT($ddl$
            CREATE TABLE IF NOT EXISTS %I (
                id           BIGSERIAL    NOT NULL,
                subject      TEXT         NOT NULL,
                payload      JSONB        NOT NULL,
                created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
                published_at TIMESTAMPTZ,
                attempts     INT          NOT NULL DEFAULT 0,
                last_error   TEXT,
                dead_at      TIMESTAMPTZ,
                PRIMARY KEY (id, created_at)
            ) PARTITION BY RANGE (created_at)
        $ddl$, tbl);

        EXECUTE FORMAT(
            'CREATE INDEX IF NOT EXISTS %I ON %I (created_at, id) WHERE published_at IS NULL AND dead_at IS NULL',
            tbl || '_unpublished_idx', tbl
        );

        FOR i IN 0..3 LOOP
            pstart := (base + (i || ' month')::INTERVAL)::DATE;
            pend   := (base + ((i + 1) || ' month')::INTERVAL)::DATE;
            pname  := tbl || '_' || TO_CHAR(pstart, 'YYYY_MM');
            EXECUTE FORMAT(
                'CREATE TABLE IF NOT EXISTS %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
                pname, tbl, pstart, pend
            );
        END LOOP;
    END LOOP;
END $$;
