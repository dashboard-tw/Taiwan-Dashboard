-- +goose Up
ALTER TABLE app.users
    ADD COLUMN email_lookup bytea CHECK (length(email_lookup) = 32);
CREATE INDEX users_email_lookup_idx ON app.users (email_lookup) WHERE email_lookup IS NOT NULL;

CREATE TABLE app.identity_link (
    token_hash  bytea PRIMARY KEY CHECK (length(token_hash) = 32),
    user_id     bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    provider    text NOT NULL,
    subject     text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);
CREATE INDEX identity_link_expires_idx ON app.identity_link (expires_at);

-- +goose Down
DROP TABLE app.identity_link;
DROP INDEX app.users_email_lookup_idx;
ALTER TABLE app.users DROP COLUMN email_lookup;
