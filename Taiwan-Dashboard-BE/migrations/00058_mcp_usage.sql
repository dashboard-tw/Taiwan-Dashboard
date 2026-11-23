-- +goose Up
CREATE TABLE app.mcp_usage_daily (
    user_id  bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    day      date NOT NULL,
    tool     text NOT NULL CHECK (char_length(tool) <= 60),
    calls    integer NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, day, tool)
);

ALTER TABLE app.mcp_usage_daily ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.mcp_usage_daily FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.mcp_usage_daily
    USING (app.rls_bypass() OR user_id = app.current_user_id());

-- +goose Down
DROP TABLE app.mcp_usage_daily;
