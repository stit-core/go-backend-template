-- name: InsertUser :one
INSERT INTO users (email, password_hash, name, status, role, email_verified)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, version, email_verified, created_at, updated_at;

-- name: GetUserByID :one
SELECT id, email, password_hash, name, status, role, version, email_verified, created_at, updated_at
FROM users
WHERE id = $1
  AND status != 'deleted';

-- name: GetUserByIDForShare :one
SELECT id, email, password_hash, name, status, role, version, email_verified, created_at, updated_at
FROM users
WHERE id = $1
  AND status != 'deleted'
FOR SHARE;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, name, status, role, version, email_verified, created_at, updated_at
FROM users
WHERE LOWER(email) = LOWER($1)
  AND status != 'deleted';

-- name: GetUserByEmailForUpdate :one
SELECT id, email, password_hash, name, status, role, version, email_verified, created_at, updated_at
FROM users
WHERE LOWER(email) = LOWER($1)
  AND status != 'deleted'
FOR UPDATE;

-- name: UpdateUser :one
UPDATE users
SET name = $1,
    status = $2,
    version = version + 1
WHERE id = $3
  AND version = $4
  AND status != 'deleted'
RETURNING id, email, password_hash, name, status, role, version, email_verified, created_at, updated_at;

-- name: DeleteUser :execrows
UPDATE users
SET status = 'deleted',
    version = version + 1
WHERE id = $1
  AND status != 'deleted';

-- name: UpdateUserPassword :one
UPDATE users
SET password_hash = $1,
    version = version + 1
WHERE id = $2
  AND version = $3
  AND status != 'deleted'
RETURNING id, email, password_hash, name, status, role, version, email_verified, created_at, updated_at;

-- name: PromoteUser :one
UPDATE users
SET role = 'admin',
    password_hash = $1,
    version = version + 1
WHERE id = $2
  AND version = $3
  AND status != 'deleted'
RETURNING id, email, password_hash, name, status, role, version, email_verified, created_at, updated_at;

-- name: SetUserEmailVerified :execrows
UPDATE users
SET email_verified = true
WHERE id = $1
  AND status != 'deleted';

-- name: ListUsers :many
SELECT id, email, password_hash, name, status, role, version, email_verified, created_at, updated_at
FROM users
WHERE status != 'deleted'
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT $1
OFFSET $2;

-- name: CountUsers :one
SELECT COUNT(*)
FROM users
WHERE status != 'deleted'
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);
