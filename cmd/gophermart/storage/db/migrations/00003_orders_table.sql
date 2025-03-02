-- +goose Up
-- +goose StatementBegin
CREATE TABLE orders (
    id            SERIAL PRIMARY KEY,
    number        BIGINT NOT NULL,
    status        TEXT NOT NULL CHECK (status IN ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED')),
    accrual       FLOAT DEFAULT 0.0,
    accrual_added BOOLEAN DEFAULT FALSE,
    uploaded_at TIMESTAMP WITH TIME ZONE NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS orders;
-- +goose StatementEnd
