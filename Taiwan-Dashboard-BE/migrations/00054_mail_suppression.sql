-- +goose Up
-- 退信或被檢舉為垃圾信的信箱，之後不再寄送。只存信箱的雜湊，不存明文。
CREATE TABLE app.mail_suppression (
    id            bigserial PRIMARY KEY,
    email_lookup  bytea NOT NULL UNIQUE,
    reason        text NOT NULL CHECK (reason IN ('hard_bounce', 'spam', 'invalid_email', 'blocked')),
    detail        text NOT NULL DEFAULT '' CHECK (char_length(detail) <= 300),
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE app.mail_suppression;
