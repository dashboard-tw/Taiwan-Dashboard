-- +goose Up
ALTER TABLE app.oauth_login
    ADD COLUMN link_user_id bigint REFERENCES app.users (id) ON DELETE CASCADE;
CREATE UNIQUE INDEX user_identity_user_provider_idx ON app.user_identity (user_id, provider);

-- +goose Down
DROP INDEX app.user_identity_user_provider_idx;
ALTER TABLE app.oauth_login DROP COLUMN link_user_id;
