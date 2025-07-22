-- +goose Up
ALTER TABLE users ADD is_chirpy_red BOOLEAN DEFAULT FALSE NOT NULL;

-- +goose Down
DROP COLUMN is_chirpy_red FROM users;