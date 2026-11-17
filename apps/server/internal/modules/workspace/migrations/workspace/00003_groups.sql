-- +goose Up
CREATE TABLE workspace_groups (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace_workspaces (id) ON DELETE CASCADE,
    name         text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX workspace_groups_name_key ON workspace_groups (workspace_id, lower(name));

-- user_id refers to a workspace member by ID only. manager_since is set
-- exactly when is_manager is true; ManagersOf orders by it.
CREATE TABLE workspace_group_members (
    group_id      uuid NOT NULL REFERENCES workspace_groups (id) ON DELETE CASCADE,
    user_id       uuid NOT NULL,
    is_manager    boolean NOT NULL DEFAULT false,
    manager_since timestamptz NULL,
    added_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id),
    CONSTRAINT workspace_group_members_manager_since CHECK (is_manager = (manager_since IS NOT NULL))
);

CREATE INDEX workspace_group_members_user_idx ON workspace_group_members (user_id);

-- +goose Down
DROP TABLE workspace_group_members;
DROP TABLE workspace_groups;
