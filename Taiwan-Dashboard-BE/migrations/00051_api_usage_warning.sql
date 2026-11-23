-- +goose Up
CREATE TABLE app.api_usage_warning (
    user_id  bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    day      date NOT NULL,
    PRIMARY KEY (user_id, day)
);

ALTER TABLE app.api_usage_warning ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.api_usage_warning FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.api_usage_warning
    USING (app.rls_bypass() OR user_id = app.current_user_id());

-- +goose Down
DROP TABLE app.api_usage_warning;
