-- +goose Up
-- +goose StatementBegin
CREATE TABLE users_balances (
    id        SERIAL,
    login     TEXT PRIMARY KEY,
    current   FLOAT DEFAULT 0.0,
    withdrawn FLOAT DEFAULT 0.0,
    FOREIGN KEY (login) REFERENCES users(login)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users_balances;
-- +goose StatementEnd
