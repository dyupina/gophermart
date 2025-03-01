-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users (
    id       SERIAL,
    uid      TEXT,
    login    TEXT PRIMARY KEY,
    password TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
