-- +goose Up

CREATE TABLE app.user_favorite (
    user_id       bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    component_id  integer REFERENCES app.component (id) ON DELETE CASCADE,
    dashboard_id  integer REFERENCES app.dashboard (id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(component_id, dashboard_id) = 1)
);
CREATE UNIQUE INDEX user_favorite_component_idx ON app.user_favorite (user_id, component_id) WHERE component_id IS NOT NULL;
CREATE UNIQUE INDEX user_favorite_dashboard_idx ON app.user_favorite (user_id, dashboard_id) WHERE dashboard_id IS NOT NULL;

CREATE TABLE app.user_dashboard (
    id              bigserial PRIMARY KEY,
    user_id         bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),
    default_county  char(5) REFERENCES ref.county (code),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_dashboard_user_idx ON app.user_dashboard (user_id);

CREATE TABLE app.user_dashboard_component (
    user_dashboard_id  bigint NOT NULL REFERENCES app.user_dashboard (id) ON DELETE CASCADE,
    component_id       integer NOT NULL REFERENCES app.component (id) ON DELETE CASCADE,
    position           smallint NOT NULL,
    size               text NOT NULL DEFAULT 'normal' CHECK (size IN ('normal', 'wide')),
    PRIMARY KEY (user_dashboard_id, component_id)
);

CREATE TABLE app.api_key (
    id            bigserial PRIMARY KEY,
    user_id       bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),
    prefix        text NOT NULL UNIQUE,
    key_hash      bytea NOT NULL UNIQUE CHECK (length(key_hash) = 32),
    tier          text NOT NULL DEFAULT 'free',
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz,
    revoked_at    timestamptz
);
CREATE INDEX api_key_user_idx ON app.api_key (user_id);

CREATE TABLE app.api_usage_daily (
    api_key_id  bigint NOT NULL REFERENCES app.api_key (id) ON DELETE CASCADE,
    day         date NOT NULL,
    requests    integer NOT NULL DEFAULT 0,
    PRIMARY KEY (api_key_id, day)
);

-- +goose StatementBegin
CREATE FUNCTION app.current_user_id() RETURNS bigint
    LANGUAGE sql STABLE
    AS $$ SELECT nullif(current_setting('app.user_id', true), '')::bigint $$;

CREATE FUNCTION app.rls_bypass() RETURNS boolean
    LANGUAGE sql STABLE
    AS $$ SELECT coalesce(current_setting('app.rls_bypass', true), '') = 'on' $$;
-- +goose StatementEnd

ALTER TABLE app.session ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.session FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.session
    USING (app.rls_bypass() OR user_id = app.current_user_id());

ALTER TABLE app.user_favorite ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.user_favorite FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.user_favorite
    USING (app.rls_bypass() OR user_id = app.current_user_id());

ALTER TABLE app.user_dashboard ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.user_dashboard FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.user_dashboard
    USING (app.rls_bypass() OR user_id = app.current_user_id());

ALTER TABLE app.user_dashboard_component ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.user_dashboard_component FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.user_dashboard_component
    USING (app.rls_bypass() OR EXISTS (
        SELECT 1 FROM app.user_dashboard d
         WHERE d.id = user_dashboard_id AND d.user_id = app.current_user_id()));

ALTER TABLE app.api_key ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.api_key FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.api_key
    USING (app.rls_bypass() OR user_id = app.current_user_id());

ALTER TABLE app.api_usage_daily ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.api_usage_daily FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.api_usage_daily
    USING (app.rls_bypass() OR EXISTS (
        SELECT 1 FROM app.api_key k
         WHERE k.id = api_key_id AND k.user_id = app.current_user_id()));

-- +goose Down
DROP POLICY owner_only ON app.session;
ALTER TABLE app.session NO FORCE ROW LEVEL SECURITY;
ALTER TABLE app.session DISABLE ROW LEVEL SECURITY;
DROP TABLE app.api_usage_daily;
DROP TABLE app.api_key;
DROP TABLE app.user_dashboard_component;
DROP TABLE app.user_dashboard;
DROP TABLE app.user_favorite;
DROP FUNCTION app.rls_bypass();
DROP FUNCTION app.current_user_id();
