-- +goose Up
ALTER TABLE app.users
    ADD COLUMN email_enc  bytea,
    ADD COLUMN email_hash bytea CHECK (length(email_hash) = 32),
    ADD CONSTRAINT users_email_pair CHECK ((email_enc IS NULL) = (email_hash IS NULL));
ALTER TABLE app.users DROP COLUMN email;

-- +goose Down
ALTER TABLE app.users ADD COLUMN email text;
ALTER TABLE app.users DROP CONSTRAINT users_email_pair, DROP COLUMN email_hash, DROP COLUMN email_enc;
