-- name: InsertEmailVerification :execrows
INSERT INTO email_verifications (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) WHERE used_at IS NULL DO NOTHING;

-- name: GetEmailVerification :one
SELECT id, user_id, expires_at, used_at
FROM email_verifications
WHERE token_hash = $1;

-- name: MarkEmailVerificationUsed :exec
UPDATE email_verifications SET used_at = NOW() WHERE id = $1;
