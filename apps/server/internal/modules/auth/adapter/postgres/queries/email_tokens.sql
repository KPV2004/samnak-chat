-- name: CreateEmailToken :exec
INSERT INTO auth_email_tokens (id, user_id, purpose, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetEmailTokenForUpdate :one
SELECT * FROM auth_email_tokens WHERE token_hash = $1 AND purpose = $2 FOR UPDATE;

-- name: MarkEmailTokenUsed :exec
UPDATE auth_email_tokens SET used_at = $2 WHERE id = $1;
