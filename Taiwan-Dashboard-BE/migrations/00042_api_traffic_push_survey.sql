-- +goose Up
ALTER TABLE app.api_usage_daily ADD COLUMN bytes bigint NOT NULL DEFAULT 0 CHECK (bytes >= 0);
ALTER TABLE app.api_survey ADD COLUMN wants_push text CHECK (wants_push IN ('yes', 'maybe', 'no'));

-- +goose Down
ALTER TABLE app.api_survey DROP COLUMN wants_push;
ALTER TABLE app.api_usage_daily DROP COLUMN bytes;
