-- +goose Up
SELECT set_config('app.rls_bypass', 'on', true);

UPDATE app.api_key SET revoked_at = now() WHERE revoked_at IS NULL;
DROP INDEX app.api_key_one_active_idx;
ALTER TABLE app.api_key ADD COLUMN disabled_at timestamptz;

CREATE TABLE app.api_token (
    token_hash  bytea PRIMARY KEY CHECK (length(token_hash) = 32),
    api_key_id  bigint NOT NULL REFERENCES app.api_key (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);
CREATE INDEX api_token_key_idx ON app.api_token (api_key_id);
CREATE INDEX api_token_expires_idx ON app.api_token (expires_at);

ALTER TABLE app.api_token ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.api_token FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.api_token
    USING (app.rls_bypass() OR EXISTS (
        SELECT 1 FROM app.api_key k
         WHERE k.id = api_key_id AND k.user_id = app.current_user_id()));

-- +goose Down
SELECT set_config('app.rls_bypass', 'on', true);
DROP TABLE app.api_token;
ALTER TABLE app.api_key DROP COLUMN disabled_at;
UPDATE app.api_key k SET revoked_at = now()
 WHERE revoked_at IS NULL
   AND EXISTS (SELECT 1 FROM app.api_key n
                WHERE n.user_id = k.user_id AND n.revoked_at IS NULL AND n.id > k.id);
CREATE UNIQUE INDEX api_key_one_active_idx ON app.api_key (user_id) WHERE revoked_at IS NULL;
