-- name: UpsertPasswordReset :one
-- Ссылка сброса запроса id: новая или новый токен неотправленной; отправленная и погашенная не трогаются —
-- RETURNING пуст.
INSERT INTO password_resets (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE
SET token_hash = EXCLUDED.token_hash
WHERE password_resets.sent_at IS NULL
  AND password_resets.used_at IS NULL
RETURNING id;

-- name: MarkPasswordResetSent :exec
UPDATE password_resets SET sent_at = NOW() WHERE id = $1;

-- name: GetPasswordReset :one
SELECT id, user_id, expires_at, used_at
FROM password_resets
WHERE token_hash = $1;

-- name: MarkPasswordResetUsed :execrows
UPDATE password_resets SET used_at = NOW() WHERE id = $1 AND used_at IS NULL;

-- name: RevokePasswordResets :exec
UPDATE password_resets SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL;
