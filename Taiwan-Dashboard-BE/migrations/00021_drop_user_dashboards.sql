-- +goose Up
DROP TABLE app.user_dashboard_component;
DROP TABLE app.user_dashboard;
DROP TABLE app.user_favorite;

-- +goose Down
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
