-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users_orders (
    id     SERIAL PRIMARY KEY,
    login  TEXT   NOT NULL,
    orders TEXT[] NOT NULL,
    FOREIGN KEY (login) REFERENCES users(login)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users_orders;
-- +goose StatementEnd
