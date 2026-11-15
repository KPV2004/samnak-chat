-- name: CreateUser :one
INSERT INTO auth_users (id, email, password_hash, display_name, avatar_url, locale, timezone, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM auth_users WHERE lower(email) = lower($1);

-- name: GetUserByID :one
SELECT * FROM auth_users WHERE id = $1;

-- name: GetUsersByIDs :many
SELECT * FROM auth_users WHERE id = ANY($1::uuid[]);

-- name: UpdateUserProfile :one
UPDATE auth_users SET
    display_name = COALESCE(sqlc.narg('display_name'), display_name),
    avatar_url   = COALESCE(sqlc.narg('avatar_url'), avatar_url),
    locale       = COALESCE(sqlc.narg('locale'), locale),
    timezone     = COALESCE(sqlc.narg('timezone'), timezone),
    updated_at   = sqlc.arg('now')
WHERE id = sqlc.arg('id')
RETURNING *;
