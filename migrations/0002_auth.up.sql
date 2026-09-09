-- 0002_auth: refresh-сессии auth-домена.
-- bruteforce_attempts владеет bruteforcekit/pgstore (источник Migrations в cmd/migrate) — здесь не дублируется.

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
