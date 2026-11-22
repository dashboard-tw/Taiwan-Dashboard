-- +goose Up
SELECT set_config('app.rls_bypass', 'on', true);

UPDATE app.api_key k SET revoked_at = now()
 WHERE revoked_at IS NULL
   AND EXISTS (SELECT 1 FROM app.api_key n
                WHERE n.user_id = k.user_id AND n.revoked_at IS NULL AND n.id > k.id);

CREATE UNIQUE INDEX api_key_one_active_idx ON app.api_key (user_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX app.api_key_one_active_idx;
