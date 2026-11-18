-- +goose Up
-- workspace_id, user IDs, channel IDs and message IDs refer to other modules'
-- rows by ID only: no foreign keys across modules. The foreign keys below stay
-- inside helpdesk.

-- HD-01, HD-04: a queue numbers its tickets. next_number is the number the
-- next ticket takes; the create transaction increments it. Prefixes are unique
-- per workspace, archived queues included, so a ticket key never repeats.
CREATE TABLE helpdesk_queues (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    name         text NOT NULL,
    prefix       text NOT NULL CHECK (prefix ~ '^[A-Z]{2,6}$'),
    channel_id   uuid,
    next_number  bigint NOT NULL DEFAULT 1,
    archived_at  timestamptz,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    UNIQUE (workspace_id, prefix)
);

-- HD-01: the agents of a queue.
CREATE TABLE helpdesk_queue_agents (
    queue_id  uuid NOT NULL REFERENCES helpdesk_queues (id) ON DELETE CASCADE,
    user_id   uuid NOT NULL,
    added_at  timestamptz NOT NULL,
    PRIMARY KEY (queue_id, user_id)
);
CREATE INDEX helpdesk_queue_agents_user_idx ON helpdesk_queue_agents (user_id, queue_id);

-- HD-02: categories belong to a queue. At most one default per workspace.
CREATE TABLE helpdesk_categories (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    queue_id     uuid NOT NULL REFERENCES helpdesk_queues (id),
    name         text NOT NULL,
    is_default   boolean NOT NULL DEFAULT false,
    archived_at  timestamptz,
    created_at   timestamptz NOT NULL
);
CREATE UNIQUE INDEX helpdesk_categories_one_default_idx ON helpdesk_categories (workspace_id)
    WHERE is_default AND archived_at IS NULL;
CREATE INDEX helpdesk_categories_queue_idx ON helpdesk_categories (queue_id);

-- HD-03 to HD-09: a ticket. The number is unique within its queue (HD-04).
-- queue_message_id is the card in the agents' channel, message_id the card in
-- the requester's channel; both stay null until the cards are posted.
CREATE TABLE helpdesk_tickets (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL,
    queue_id          uuid NOT NULL REFERENCES helpdesk_queues (id),
    category_id       uuid NOT NULL REFERENCES helpdesk_categories (id),
    number            bigint NOT NULL,
    title             text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description       text NOT NULL DEFAULT '' CHECK (char_length(description) <= 8000),
    requester_id      uuid NOT NULL,
    assignee_id       uuid,
    status            text NOT NULL DEFAULT 'new'
                      CHECK (status IN ('new', 'open', 'in_progress', 'waiting', 'resolved', 'closed')),
    priority          text NOT NULL DEFAULT 'normal'
                      CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    source_channel_id uuid,
    source_message_id uuid,
    message_id        uuid,
    queue_message_id  uuid,
    resolved_at       timestamptz,
    closed_at         timestamptz,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL,
    UNIQUE (queue_id, number)
);
-- HD-10: "my requests", the queue view and the assignee filter, newest first.
CREATE INDEX helpdesk_tickets_requester_idx ON helpdesk_tickets (workspace_id, requester_id, id DESC);
CREATE INDEX helpdesk_tickets_queue_idx ON helpdesk_tickets (workspace_id, queue_id, status, id DESC);
CREATE INDEX helpdesk_tickets_assignee_idx ON helpdesk_tickets (workspace_id, assignee_id, id DESC) WHERE assignee_id IS NOT NULL;

-- HD-07: public comments (internal = false) and internal notes.
CREATE TABLE helpdesk_comments (
    id         uuid PRIMARY KEY,
    ticket_id  uuid NOT NULL REFERENCES helpdesk_tickets (id) ON DELETE CASCADE,
    author_id  uuid NOT NULL,
    body       text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 8000),
    internal   boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL
);
CREATE INDEX helpdesk_comments_ticket_idx ON helpdesk_comments (ticket_id, created_at, id);

-- HD-18: every change of a ticket. Rows are only ever added. internal rows are
-- hidden from the requester, like internal comments.
CREATE TABLE helpdesk_history (
    id          uuid PRIMARY KEY,
    ticket_id   uuid NOT NULL REFERENCES helpdesk_tickets (id) ON DELETE CASCADE,
    actor_id    uuid NOT NULL,
    action      text NOT NULL CHECK (action IN (
                    'created', 'assigned', 'status_changed', 'priority_changed', 'category_changed', 'commented')),
    from_value  text NOT NULL DEFAULT '',
    to_value    text NOT NULL DEFAULT '',
    internal    boolean NOT NULL DEFAULT false,
    at          timestamptz NOT NULL
);
CREATE INDEX helpdesk_history_ticket_idx ON helpdesk_history (ticket_id, at, id);

-- HD-03, HD-01: a file exists before its ticket does, so ticket_id stays null
-- until submit links it. object_key is helpdesk/{workspace_id}/{id} for uploads
-- and chat's own key for files copied from a message (from_message = true).
CREATE TABLE helpdesk_attachments (
    id           uuid PRIMARY KEY,
    ticket_id    uuid REFERENCES helpdesk_tickets (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL,
    uploader_id  uuid NOT NULL,
    object_key   text NOT NULL,
    file_name    text NOT NULL,
    size         bigint NOT NULL,
    mime_type    text NOT NULL,
    from_message boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL
);
CREATE INDEX helpdesk_attachments_ticket_idx ON helpdesk_attachments (ticket_id, created_at, id) WHERE ticket_id IS NOT NULL;

-- +goose Down
DROP TABLE helpdesk_attachments;
DROP TABLE helpdesk_history;
DROP TABLE helpdesk_comments;
DROP TABLE helpdesk_tickets;
DROP TABLE helpdesk_categories;
DROP TABLE helpdesk_queue_agents;
DROP TABLE helpdesk_queues;
