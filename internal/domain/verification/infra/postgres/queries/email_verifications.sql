-- name: UpsertEmailVerification :one
-- Ожидающее подтверждение пользователя одно (партиал-индекс): неотправленное получает новый токен,
-- отправленное не трогается — RETURNING пуст.
INSERT INTO email_verifications (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) WHERE used_at IS NULL DO UPDATE
SET token_hash = EXCLUDED.token_hash,
    expires_at = EXCLUDED.expires_at
WHERE email_verifications.sent_at IS NULL
RETURNING id;

-- name: MarkEmailVerificationSent :exec
UPDATE email_verifications SET sent_at = NOW() WHERE id = $1;

-- name: GetEmailVerification :one
SELECT id, user_id, expires_at, used_at
FROM email_verifications
WHERE token_hash = $1;

-- name: MarkEmailVerificationUsed :execrows
UPDATE email_verifications SET used_at = NOW() WHERE id = $1 AND used_at IS NULL;
