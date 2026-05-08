-- name: InsertRefreshSession :exec
INSERT INTO refresh_sessions (jti, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: GetRefreshSession :one
SELECT jti, user_id, issued_at, expires_at, used_at, replaced_by_jti
FROM refresh_sessions
WHERE jti = $1;

-- name: MarkRefreshSessionUsed :exec
UPDATE refresh_sessions
SET used_at = NOW(),
    replaced_by_jti = $2
WHERE jti = $1;
