-- name: CreateDevice :one
INSERT INTO auth_devices (id, user_id, name, platform, last_seen_at, created_at)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING *;

-- name: GetActiveDevice :one
SELECT * FROM auth_devices WHERE id = $1 AND revoked_at IS NULL;

-- name: TouchDevice :exec
UPDATE auth_devices SET last_seen_at = $2 WHERE id = $1;

-- name: ListActiveDevices :many
SELECT * FROM auth_devices
WHERE user_id = $1 AND revoked_at IS NULL
ORDER BY last_seen_at DESC, id DESC;

-- name: RevokeDevice :execrows
UPDATE auth_devices SET revoked_at = $3
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: RevokeDeviceByID :exec
UPDATE auth_devices SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeUserDevices :exec
UPDATE auth_devices SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL;
