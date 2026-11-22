-- +goose Up

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dashboard_reader') THEN
        RAISE EXCEPTION '找不到資料庫帳號 dashboard_reader，請先用超級使用者執行 CREATE ROLE dashboard_reader NOLOGIN';
    END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA ref, data TO dashboard_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA ref, data TO dashboard_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA ref, data GRANT SELECT ON TABLES TO dashboard_reader;

CREATE TABLE app.component (
    id              serial PRIMARY KEY,
    slug            text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name            text NOT NULL,
    topic           text NOT NULL CHECK (topic IN ('aqi', 'reservoir', 'power')),
    short_desc      text NOT NULL DEFAULT '',
    long_desc       text NOT NULL DEFAULT '',
    use_case        text NOT NULL DEFAULT '',
    dataset_ids     text[] NOT NULL DEFAULT '{}',   -- 關聯 app.dataset.id，匯入時檢查
    time_range      text NOT NULL DEFAULT 'current'
                    CHECK (time_range IN ('current', 'day', 'week', 'month', 'quarter',
                                          'halfyear', 'year', 'twoyear', 'fiveyear')),
    chart_types     text[] NOT NULL CHECK (cardinality(chart_types) > 0),
    colors          text[] NOT NULL DEFAULT '{}',
    unit            text NOT NULL DEFAULT '',
    decimals        smallint NOT NULL DEFAULT 0 CHECK (decimals BETWEEN 0 AND 6),
    chart_map_link  jsonb,
    links           text[] NOT NULL DEFAULT '{}',
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.component_query (
    component_id  integer NOT NULL REFERENCES app.component (id) ON DELETE CASCADE,
    scope         text NOT NULL CHECK (scope IN ('national', 'county')),
    data_shape    text NOT NULL
                  CHECK (data_shape IN ('by_county', 'two_d', 'three_d', 'percent', 'time', 'map_legend')),
    chart_sql     text NOT NULL,
    history_sql   text,
    PRIMARY KEY (component_id, scope)
);

CREATE TABLE app.component_map_layer (
    id            serial PRIMARY KEY,
    component_id  integer NOT NULL REFERENCES app.component (id) ON DELETE CASCADE,
    sort_order    smallint NOT NULL DEFAULT 0,
    title         text NOT NULL,
    layer_type    text NOT NULL CHECK (layer_type IN ('circle', 'symbol', 'fill', 'line', 'heatmap')),
    source_type   text NOT NULL CHECK (source_type IN ('county_boundary', 'query')),
    geojson_sql   text,
    style         jsonb NOT NULL DEFAULT '{}',
    popup_fields  jsonb NOT NULL DEFAULT '[]',
    CHECK (source_type <> 'query' OR geojson_sql IS NOT NULL)
);

CREATE TABLE app.component_version (
    id            bigserial PRIMARY KEY,
    component_id  integer NOT NULL REFERENCES app.component (id) ON DELETE CASCADE,
    snapshot      jsonb NOT NULL,
    note          text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    created_by    integer                           -- 之後加上管理員帳號的外鍵
);
CREATE INDEX component_version_component_idx ON app.component_version (component_id, id DESC);

CREATE TABLE app.dashboard (
    id           serial PRIMARY KEY,
    slug         text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name         text NOT NULL,
    icon         text NOT NULL DEFAULT '',
    description  text NOT NULL DEFAULT '',
    sort_order   smallint NOT NULL DEFAULT 0
);

CREATE TABLE app.dashboard_component (
    dashboard_id  integer NOT NULL REFERENCES app.dashboard (id) ON DELETE CASCADE,
    component_id  integer NOT NULL REFERENCES app.component (id),
    position      smallint NOT NULL,
    size          text NOT NULL DEFAULT 'normal' CHECK (size IN ('normal', 'wide')),
    PRIMARY KEY (dashboard_id, component_id)
);

-- +goose Down
DROP TABLE app.dashboard_component;
DROP TABLE app.dashboard;
DROP TABLE app.component_version;
DROP TABLE app.component_map_layer;
DROP TABLE app.component_query;
DROP TABLE app.component;
ALTER DEFAULT PRIVILEGES IN SCHEMA ref, data REVOKE SELECT ON TABLES FROM dashboard_reader;
REVOKE SELECT ON ALL TABLES IN SCHEMA ref, data FROM dashboard_reader;
REVOKE USAGE ON SCHEMA ref, data FROM dashboard_reader;
