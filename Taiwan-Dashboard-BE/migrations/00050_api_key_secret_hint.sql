-- +goose Up
ALTER TABLE app.api_key ADD COLUMN secret_hint text CHECK (char_length(secret_hint) <= 40);

-- +goose Down
ALTER TABLE app.api_key DROP COLUMN secret_hint;
