-- +goose Up
CREATE TABLE app.oauth_client (
    id             text PRIMARY KEY CHECK (char_length(id) BETWEEN 1 AND 500),
    name           text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    client_uri     text NOT NULL DEFAULT '' CHECK (char_length(client_uri) <= 500),
    redirect_uris  text[] NOT NULL CHECK (cardinality(redirect_uris) BETWEEN 1 AND 10),
    auth_method    text NOT NULL DEFAULT 'none'
                   CHECK (auth_method IN ('none', 'client_secret_post', 'client_secret_basic')),
    secret_hash    bytea CHECK (secret_hash IS NULL OR length(secret_hash) = 32),
    metadata_doc   boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    fetched_at     timestamptz,
    last_used_at   timestamptz
);
CREATE INDEX oauth_client_unused_idx ON app.oauth_client (coalesce(last_used_at, created_at));

ALTER TABLE app.oauth_client ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.oauth_client FORCE ROW LEVEL SECURITY;
CREATE POLICY system_only ON app.oauth_client USING (app.rls_bypass());

ALTER TABLE app.api_key
    ADD COLUMN kind text NOT NULL DEFAULT 'client' CHECK (kind IN ('client', 'oauth')),
    ADD COLUMN oauth_client_id text REFERENCES app.oauth_client (id) ON DELETE CASCADE,
    ADD CONSTRAINT api_key_oauth_client CHECK ((kind = 'oauth') = (oauth_client_id IS NOT NULL));
CREATE UNIQUE INDEX api_key_oauth_grant_idx ON app.api_key (user_id, oauth_client_id)
    WHERE kind = 'oauth' AND revoked_at IS NULL;

CREATE TABLE app.oauth_code (
    code_hash       bytea PRIMARY KEY CHECK (length(code_hash) = 32),
    client_id       text NOT NULL REFERENCES app.oauth_client (id) ON DELETE CASCADE,
    user_id         bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    redirect_uri    text NOT NULL,
    code_challenge  text NOT NULL,
    scope           text NOT NULL,
    resource        text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL
);
CREATE INDEX oauth_code_expires_idx ON app.oauth_code (expires_at);

ALTER TABLE app.oauth_code ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.oauth_code FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.oauth_code
    USING (app.rls_bypass() OR user_id = app.current_user_id());

CREATE TABLE app.oauth_refresh (
    token_hash  bytea PRIMARY KEY CHECK (length(token_hash) = 32),
    api_key_id  bigint NOT NULL REFERENCES app.api_key (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);
CREATE INDEX oauth_refresh_key_idx ON app.oauth_refresh (api_key_id);
CREATE INDEX oauth_refresh_expires_idx ON app.oauth_refresh (expires_at);

ALTER TABLE app.oauth_refresh ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.oauth_refresh FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.oauth_refresh
    USING (app.rls_bypass() OR EXISTS (
        SELECT 1 FROM app.api_key k
         WHERE k.id = api_key_id AND k.user_id = app.current_user_id()));

-- +goose Down
SELECT set_config('app.rls_bypass', 'on', true);
DROP TABLE app.oauth_refresh;
DROP TABLE app.oauth_code;
DELETE FROM app.api_key WHERE kind = 'oauth';
DROP INDEX app.api_key_oauth_grant_idx;
ALTER TABLE app.api_key DROP CONSTRAINT api_key_oauth_client,
    DROP COLUMN oauth_client_id, DROP COLUMN kind;
DROP TABLE app.oauth_client;
