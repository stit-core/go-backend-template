-- name: InsertPasswordReset :one
INSERT INTO password_resets (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id;

-- name: GetPasswordReset :one
SELECT id, user_id, expires_at, used_at
FROM password_resets
WHERE token_hash = $1;

-- name: MarkPasswordResetUsed :execrows
UPDATE password_resets SET used_at = NOW() WHERE id = $1 AND used_at IS NULL;

-- name: RevokePasswordResets :exec
UPDATE password_resets SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL;
