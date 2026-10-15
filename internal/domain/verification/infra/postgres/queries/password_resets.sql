-- name: UpsertPasswordReset :one
-- Ссылка сброса запроса id с версией пользователя на момент письма: новая или новый токен своей неотправленной.
-- Отправленную, погашенную и чужую не трогает; не выпускает ссылку запросу, после которого пользователь погасил
-- ссылку сброса, и пока у пользователя есть другая ссылка моложе паузы писем. Не выпущено — RETURNING пуст.
INSERT INTO password_resets (id, user_id, token_hash, expires_at, user_version)
SELECT sqlc.arg('id')::uuid, sqlc.arg('user_id')::uuid, sqlc.arg('token_hash')::text, sqlc.arg('expires_at')::timestamptz,
       sqlc.arg('user_version')::int
WHERE NOT EXISTS (
    SELECT 1 FROM password_resets
    WHERE user_id = sqlc.arg('user_id')::uuid
      AND (used_at > sqlc.arg('requested_at')::timestamptz
           OR id <> sqlc.arg('id')::uuid AND created_at > NOW() - sqlc.arg('mail_pause')::interval)
)
ON CONFLICT (id) DO UPDATE
SET token_hash = EXCLUDED.token_hash,
    user_version = EXCLUDED.user_version
WHERE password_resets.user_id = EXCLUDED.user_id
  AND password_resets.sent_at IS NULL
  AND password_resets.used_at IS NULL
RETURNING id;

-- name: LockPasswordResets :exec
-- Выпуски ссылок одного пользователя — по очереди до конца транзакции: пауза писем видит ссылку соседнего выпуска.
SELECT pg_advisory_xact_lock(hashtext('verification.password_resets'), hashtext(sqlc.arg('user_id')::text));

-- name: MarkPasswordResetSent :exec
UPDATE password_resets SET sent_at = NOW() WHERE id = $1;

-- name: GetPasswordReset :one
SELECT id, user_id, user_version, expires_at, used_at
FROM password_resets
WHERE token_hash = $1;

-- name: RedeemPasswordResets :many
-- Все непогашенные ссылки пользователя — одним оператором: строки блокируются в одном порядке, и подтверждения
-- двух ссылок одного пользователя не ждут друг друга по кругу (40P01).
UPDATE password_resets SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL RETURNING id;

-- name: DeleteExpiredLinks :one
-- Ссылки сброса и подтверждения адреса, истёкшие раньше before, — батчем: на разросшейся таблице безлимитный
-- DELETE не уложился бы в PG_STATEMENT_TIMEOUT.
WITH resets AS (
    DELETE FROM password_resets
    WHERE id IN (SELECT id FROM password_resets WHERE expires_at < sqlc.arg('before')::timestamptz LIMIT 5000)
    RETURNING id
), verifications AS (
    DELETE FROM email_verifications
    WHERE id IN (SELECT id FROM email_verifications WHERE expires_at < sqlc.arg('before')::timestamptz LIMIT 5000)
    RETURNING id
)
SELECT ((SELECT count(*) FROM resets) + (SELECT count(*) FROM verifications))::bigint AS deleted;
