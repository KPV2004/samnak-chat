-- +goose Up
CREATE TABLE platform_outbox_events (
    id           uuid PRIMARY KEY,
    name         text NOT NULL,
    payload      jsonb NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz NULL
);

CREATE INDEX platform_outbox_events_unpublished_idx
    ON platform_outbox_events (created_at) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE platform_outbox_events;
