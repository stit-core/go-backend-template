DO $$
DECLARE
    tbl TEXT;
BEGIN
    FOREACH tbl IN ARRAY ARRAY['outbox_user', 'outbox_verification'] LOOP
        EXECUTE FORMAT('DROP TABLE IF EXISTS %I', tbl);
    END LOOP;
END $$;
