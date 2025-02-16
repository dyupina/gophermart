-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    uid      VARCHAR(255) PRIMARY KEY,
    login    VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
