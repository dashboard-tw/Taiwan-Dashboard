-- +goose Up
ALTER TABLE app.session
    ADD COLUMN id           bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    ADD COLUMN last_seen_at timestamptz;
UPDATE app.session SET last_seen_at = created_at;
CREATE INDEX session_expires_idx ON app.session (expires_at);
CREATE INDEX audit_log_ip_idx ON app.audit_log (created_at) WHERE ip IS NOT NULL;

-- +goose Down
DROP INDEX app.audit_log_ip_idx;
DROP INDEX app.session_expires_idx;
ALTER TABLE app.session DROP COLUMN last_seen_at, DROP COLUMN id;
