-- +goose Up
-- +goose StatementBegin
CREATE TABLE users_withdrawals (
    id           SERIAL PRIMARY KEY,
    login        TEXT,
    order_number BIGINT,
    sum          FLOAT DEFAULT 0.0,
    processed_at TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (login) REFERENCES users_balances(login)
);
-- FOREIGN KEY (order_number) REFERENCES orders(number)

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users_withdrawals;
-- +goose StatementEnd
