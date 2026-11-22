-- +goose Up

CREATE TABLE app.users (
    id             bigserial PRIMARY KEY,
    email          text,                            -- 可為 NULL：Apple 允許使用者隱藏 email
    display_name   text NOT NULL,
    role           text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    status         text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_login_at  timestamptz
);

CREATE TABLE app.user_identity (
    user_id     bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    provider    text NOT NULL CHECK (provider IN ('dev', 'apple', 'gov')),
    subject     text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);
CREATE INDEX user_identity_user_idx ON app.user_identity (user_id);

CREATE TABLE app.session (
    id_hash     bytea PRIMARY KEY CHECK (length(id_hash) = 32),
    user_id     bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    ip          inet,
    user_agent  text
);
CREATE INDEX session_user_idx ON app.session (user_id);

CREATE TABLE app.audit_log (
    id          bigserial PRIMARY KEY,
    user_id     bigint REFERENCES app.users (id) ON DELETE SET NULL,
    action      text NOT NULL,                      -- login、logout、component.update…
    target      text NOT NULL DEFAULT '',
    detail      jsonb NOT NULL DEFAULT '{}',
    ip          inet,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_created_idx ON app.audit_log (created_at DESC);

ALTER TABLE app.component_version ALTER COLUMN created_by TYPE bigint;
ALTER TABLE app.component_version
    ADD CONSTRAINT component_version_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES app.users (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE app.component_version DROP CONSTRAINT component_version_created_by_fkey;
ALTER TABLE app.component_version ALTER COLUMN created_by TYPE integer;
DROP TABLE app.audit_log;
DROP TABLE app.session;
DROP TABLE app.user_identity;
DROP TABLE app.users;
