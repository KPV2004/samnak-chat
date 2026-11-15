-- +goose Up
CREATE TABLE auth_users (
    id            uuid PRIMARY KEY,
    email         text NOT NULL,
    password_hash text NOT NULL,
    display_name  text NOT NULL,
    avatar_url    text NOT NULL DEFAULT '',
    locale        text NOT NULL DEFAULT 'th' CHECK (locale IN ('th', 'en')),
    timezone      text NOT NULL DEFAULT 'Asia/Bangkok',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Emails are stored lower-case by the domain; the index also enforces it.
CREATE UNIQUE INDEX auth_users_email_key ON auth_users (lower(email));

-- One row per login. The device ID is the session.
CREATE TABLE auth_devices (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES auth_users (id) ON DELETE CASCADE,
    name         text NOT NULL,
    platform     text NOT NULL CHECK (platform IN ('web', 'desktop', 'ios', 'android')),
    push_token   text NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz NULL
);

CREATE INDEX auth_devices_user_idx ON auth_devices (user_id) WHERE revoked_at IS NULL;

-- Only the SHA-256 of the refresh token is stored.
CREATE TABLE auth_refresh_tokens (
    id         uuid PRIMARY KEY,
    device_id  uuid NOT NULL REFERENCES auth_devices (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz NULL
);

CREATE INDEX auth_refresh_tokens_device_idx ON auth_refresh_tokens (device_id);

-- +goose Down
DROP TABLE auth_refresh_tokens;
DROP TABLE auth_devices;
DROP TABLE auth_users;
