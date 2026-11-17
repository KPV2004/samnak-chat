-- +goose Up
-- workspace_id and every user ID refer to other modules' rows by ID only: no
-- foreign keys across modules. channel_id and message_id point at the card in
-- the requester-approver DM; they stay null until the card is posted.
CREATE TABLE approval_requests (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    type         text NOT NULL CHECK (type IN ('leave', 'general')),
    requester_id uuid NOT NULL,
    approver_id  uuid NOT NULL,
    status       text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    payload      jsonb NOT NULL,
    channel_id   uuid,
    message_id   uuid,
    decided_at   timestamptz,
    reason       text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL
);

-- APR-06: "my requests" and "waiting for me", newest first.
CREATE INDEX approval_requests_requester_idx ON approval_requests (workspace_id, requester_id, created_at DESC, id DESC);
CREATE INDEX approval_requests_approver_idx ON approval_requests (workspace_id, approver_id, created_at DESC, id DESC);

-- One step in v0.1; several ordered steps are APR-11.
CREATE TABLE approval_steps (
    request_id  uuid NOT NULL REFERENCES approval_requests (id) ON DELETE CASCADE,
    step_no     integer NOT NULL,
    approver_id uuid NOT NULL,
    status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    reason      text NOT NULL DEFAULT '',
    decided_at  timestamptz,
    PRIMARY KEY (request_id, step_no)
);

-- APR-07: who did what, when and why. Rows are only ever added.
CREATE TABLE approval_history (
    id         uuid PRIMARY KEY,
    request_id uuid NOT NULL REFERENCES approval_requests (id) ON DELETE CASCADE,
    actor_id   uuid NOT NULL,
    action     text NOT NULL CHECK (action IN ('created', 'approved', 'rejected', 'cancelled')),
    reason     text NOT NULL DEFAULT '',
    at         timestamptz NOT NULL
);
CREATE INDEX approval_history_request_idx ON approval_history (request_id, at, id);

-- APR-09: a file exists before its request does, so request_id is null until
-- submit links it. object_key is approval/{workspace_id}/{id}.
CREATE TABLE approval_attachments (
    id           uuid PRIMARY KEY,
    request_id   uuid REFERENCES approval_requests (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL,
    uploader_id  uuid NOT NULL,
    object_key   text NOT NULL,
    file_name    text NOT NULL,
    size         bigint NOT NULL,
    mime_type    text NOT NULL,
    created_at   timestamptz NOT NULL
);
CREATE INDEX approval_attachments_request_idx ON approval_attachments (request_id, created_at, id) WHERE request_id IS NOT NULL;

-- +goose Down
DROP TABLE approval_attachments;
DROP TABLE approval_history;
DROP TABLE approval_steps;
DROP TABLE approval_requests;
