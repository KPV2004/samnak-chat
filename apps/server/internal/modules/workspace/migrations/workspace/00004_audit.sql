-- +goose Up
-- Admin actions of a workspace (WSP-10). Rows are written in the same
-- transaction as the change they describe, so a change and its row exist
-- together or not at all. actor_id and target_id refer to users or other
-- rows by ID only, as elsewhere in this module.
CREATE TABLE workspace_audit (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace_workspaces (id) ON DELETE CASCADE,
    actor_id     uuid NOT NULL,
    action       text NOT NULL,
    target_type  text NOT NULL,
    target_id    uuid NOT NULL,
    data         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT workspace_audit_target_type_valid CHECK (target_type IN ('member', 'invite', 'group'))
);

-- The log is read newest first, one workspace at a time.
CREATE INDEX workspace_audit_workspace_idx ON workspace_audit (workspace_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE workspace_audit;
