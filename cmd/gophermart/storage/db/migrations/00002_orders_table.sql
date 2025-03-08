-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS orders (
    id            SERIAL PRIMARY KEY,
    login         TEXT NOT NULL,
    number        BIGINT NOT NULL UNIQUE,
    status        TEXT NOT NULL CHECK (status IN ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED')),
    accrual       FLOAT DEFAULT 0.0,
    accrual_added BOOLEAN DEFAULT FALSE,
    uploaded_at   TIMESTAMP WITH TIME ZONE NOT NULL,
    FOREIGN KEY (login) REFERENCES users(login)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS orders;
-- +goose StatementEnd
