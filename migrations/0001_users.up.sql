-- 0001_users: пользователи + email-верификация и сброс пароля.
-- Покрывает домены user (root) и verification (email tokens, password resets).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email          TEXT        NOT NULL,
    password_hash  TEXT        NOT NULL,
    name           TEXT        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'active',
    role           TEXT        NOT NULL DEFAULT 'client',
    version        INT         NOT NULL DEFAULT 1,
    email_verified BOOLEAN     NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_status_check CHECK (status IN ('active', 'blocked', 'deleted')),
    CONSTRAINT users_role_check   CHECK (role   IN ('admin',  'client'))
);

CREATE UNIQUE INDEX users_email_uq   ON users (LOWER(email)) WHERE status != 'deleted';
CREATE INDEX        users_status_idx ON users (status);
CREATE INDEX        users_role_idx   ON users (role);

-- Общая trigger-функция: обновляет updated_at на UPDATE.
-- Используется и другими таблицами: подключай через CREATE TRIGGER ... EXECUTE FUNCTION set_updated_at().
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Сброс пароля (verification-домен).
CREATE TABLE password_resets (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT         NOT NULL,
    expires_at  TIMESTAMPTZ  NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX        password_resets_user_id_idx    ON password_resets (user_id);
CREATE UNIQUE INDEX password_resets_token_hash_idx ON password_resets (token_hash);

-- Верификация email (verification-домен).
CREATE TABLE email_verifications (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT         NOT NULL,
    expires_at  TIMESTAMPTZ  NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX email_verifications_token_hash_idx ON email_verifications (token_hash);
