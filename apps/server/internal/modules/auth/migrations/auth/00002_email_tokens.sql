-- +goose Up
ALTER TABLE auth_users ADD COLUMN email_verified_at timestamptz NULL;

-- Links sent by mail: verify the address or reset the password (AUTH-09).
-- Only the SHA-256 of the token is stored.
CREATE TABLE auth_email_tokens (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES auth_users (id) ON DELETE CASCADE,
    purpose    text NOT NULL CHECK (purpose IN ('verify', 'reset')),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX auth_email_tokens_user_idx ON auth_email_tokens (user_id);

-- +goose Down
DROP TABLE auth_email_tokens;
ALTER TABLE auth_users DROP COLUMN email_verified_at;
