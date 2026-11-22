-- +goose Up
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    object_key TEXT NOT NULL UNIQUE,
    status VARCHAR NOT NULL DEFAULT 'created',
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE TABLE outbox (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    task_id UUID NOT NULL UNIQUE REFERENCES tasks(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    published_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE outbox;
DROP TABLE tasks;