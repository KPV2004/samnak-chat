-- name: CreateRefreshToken :exec
INSERT INTO auth_refresh_tokens (id, device_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetRefreshTokenForUpdate :one
SELECT * FROM auth_refresh_tokens WHERE token_hash = $1 FOR UPDATE;

-- name: RevokeRefreshToken :exec
UPDATE auth_refresh_tokens SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeDeviceRefreshTokens :exec
UPDATE auth_refresh_tokens SET revoked_at = $2 WHERE device_id = $1 AND revoked_at IS NULL;

-- name: RevokeUserRefreshTokens :exec
UPDATE auth_refresh_tokens SET revoked_at = $2
WHERE revoked_at IS NULL AND device_id IN (SELECT id FROM auth_devices WHERE user_id = $1);
