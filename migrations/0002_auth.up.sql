-- 0002_auth: refresh-сессии и счётчики bruteforce.
-- Покрывает auth-домен и общую инфраструктуру bruteforcekit (используется auth login flow).

-- Refresh-токены: jti как PK, replaced_by_jti для chain rotation.
CREATE TABLE refresh_sessions (
    jti              UUID         PRIMARY KEY,
    user_id          UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issued_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at       TIMESTAMPTZ  NOT NULL,
    used_at          TIMESTAMPTZ,
    replaced_by_jti  UUID
);
CREATE INDEX refresh_sessions_user_id_idx    ON refresh_sessions (user_id);
CREATE INDEX refresh_sessions_expires_at_idx ON refresh_sessions (expires_at);

-- Bruteforce-счётчики: PK по (subject, ip), partial index на активные locks.
CREATE TABLE bruteforce_attempts (
    subject       TEXT        NOT NULL,
    ip            TEXT        NOT NULL,
    failed_count  INT         NOT NULL DEFAULT 0,
    locked_until  TIMESTAMPTZ,
    last_attempt  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subject, ip)
);
CREATE INDEX bruteforce_attempts_locked_idx ON bruteforce_attempts (locked_until) WHERE locked_until IS NOT NULL;
