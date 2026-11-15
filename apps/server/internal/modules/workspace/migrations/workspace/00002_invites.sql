-- +goose Up
CREATE TABLE workspace_invites (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace_workspaces (id) ON DELETE CASCADE,
    email        text NULL,
    token_hash   text NOT NULL,
    role         text NOT NULL,
    created_by   uuid NOT NULL,
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz NULL,
    used_at      timestamptz NULL,
    used_by      uuid NULL,
    CONSTRAINT workspace_invites_role_valid CHECK (role IN ('admin', 'manager', 'member', 'guest'))
);

CREATE UNIQUE INDEX workspace_invites_token_hash_key ON workspace_invites (token_hash);
CREATE INDEX workspace_invites_workspace_idx ON workspace_invites (workspace_id, created_at DESC);

-- +goose Down
DROP TABLE workspace_invites;
