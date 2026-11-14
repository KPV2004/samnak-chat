-- +goose Up
CREATE TABLE demo_things (id uuid PRIMARY KEY);
-- +goose Down
DROP TABLE demo_things;
