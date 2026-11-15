-- +goose Up
CREATE TABLE workspace_workspaces (
    id         uuid PRIMARY KEY,
    name       text NOT NULL,
    slug       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT workspace_workspaces_slug_format CHECK (slug ~ '^[a-z0-9-]{3,40}$')
);

CREATE UNIQUE INDEX workspace_workspaces_slug_key ON workspace_workspaces (slug);

-- user_id refers to auth_users by ID only: no foreign key across modules.
CREATE TABLE workspace_members (
    workspace_id uuid NOT NULL REFERENCES workspace_workspaces (id) ON DELETE CASCADE,
    user_id      uuid NOT NULL,
    role         text NOT NULL,
    status       text NOT NULL DEFAULT 'active',
    joined_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id),
    CONSTRAINT workspace_members_role_valid CHECK (role IN ('owner', 'admin', 'manager', 'member', 'guest')),
    CONSTRAINT workspace_members_status_valid CHECK (status IN ('active', 'deactivated'))
);

CREATE INDEX workspace_members_user_idx ON workspace_members (user_id);

-- +goose Down
DROP TABLE workspace_members;
DROP TABLE workspace_workspaces;
