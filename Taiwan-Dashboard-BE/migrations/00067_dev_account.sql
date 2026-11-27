-- +goose Up

-- 開發用帳號：不用填使用調查、沒有憑證數與額度限制，後台統計不算這個帳號
ALTER TABLE app.users ADD COLUMN is_dev boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE app.users DROP COLUMN is_dev;
