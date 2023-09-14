-- +goose Up
CREATE TABLE llo_streams (
    id text PRIMARY KEY,
    pipeline_spec_id INT REFENCES (pipeline_specs) id,
    created_at timestamp with time zone NOT NULL
);

-- +goose Down
DROP TABLE llo_streams;
