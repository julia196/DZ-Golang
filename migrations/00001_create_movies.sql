-- +goose Up
CREATE TABLE IF NOT EXISTS movies (
    id         INTEGER PRIMARY KEY, -- id со стенда
    title      TEXT NOT NULL,
    year       INTEGER NOT NULL,
    director   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS movies;