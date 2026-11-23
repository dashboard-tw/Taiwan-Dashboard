-- +goose Up
CREATE TABLE data.debris_alert (
    alert_type     text NOT NULL CHECK (alert_type IN ('D', 'L')),
    alert_id       text NOT NULL,                 -- 土石流潛勢溪流編號，或崩塌區編號
    name           text,                          -- 崩塌區名稱
    county_code    char(5) REFERENCES ref.county (code),
    town           text,
    village        text,
    level          text NOT NULL CHECK (level IN ('yellow', 'red')),
    updated_at     timestamptz,                   -- 來源的更新時間
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (alert_type, alert_id, first_seen_at)
);
CREATE INDEX debris_alert_seen_idx ON data.debris_alert (last_seen_at);
CREATE VIEW data.debris_alert_active AS
    SELECT * FROM data.debris_alert WHERE last_seen_at >= now() - interval '30 minutes';

CREATE TABLE ref.flood_sensor (
    station_code   text PRIMARY KEY,
    name           text NOT NULL,
    authority      text,
    county_code    char(5) REFERENCES ref.county (code),
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX flood_sensor_county_idx ON ref.flood_sensor (county_code);
CREATE TABLE data.flood_current (
    station_code   text PRIMARY KEY REFERENCES ref.flood_sensor (station_code),
    depth_cm       numeric(6, 1) NOT NULL,
    observed_at    timestamptz NOT NULL,
    fetched_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE data.flood_obs (
    station_code   text NOT NULL REFERENCES ref.flood_sensor (station_code),
    observed_at    timestamptz NOT NULL,
    depth_cm       numeric(6, 1) NOT NULL,
    PRIMARY KEY (station_code, observed_at)
);
CREATE INDEX flood_obs_time_idx ON data.flood_obs (observed_at);
CREATE TABLE data.flood_county_history (
    observed_at    timestamptz NOT NULL,
    county_code    char(5) NOT NULL REFERENCES ref.county (code),
    sensors        integer NOT NULL,
    flooded        integer NOT NULL,
    max_depth_cm   numeric(6, 1),
    PRIMARY KEY (observed_at, county_code)
);

CREATE TABLE ref.ev_station (
    station_id       text PRIMARY KEY,
    name             text NOT NULL,
    county_code      char(5) REFERENCES ref.county (code),
    operation_type   smallint,
    spaces           integer,
    charging_points  integer,
    ac_connectors    integer NOT NULL,
    dc_connectors    integer NOT NULL,
    connector_types  text[] NOT NULL DEFAULT '{}', -- 例如 J1772、CCS2
    service_time     text,
    charging_rate    text,
    parking_rate     text,
    geom             geometry(Point, 4326),
    first_seen_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ev_station_county_idx ON ref.ev_station (county_code);
CREATE TABLE data.ev_station_live (
    station_id   text PRIMARY KEY,
    free         integer NOT NULL,
    in_use       integer NOT NULL,
    broken       integer NOT NULL,
    updated_at   timestamptz,
    fetched_at   timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE data.ev_county_history (
    observed_at  timestamptz NOT NULL,
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    free         integer NOT NULL,
    in_use       integer NOT NULL,
    broken       integer NOT NULL,
    PRIMARY KEY (observed_at, county_code)
);

CREATE TABLE data.poultry_price (
    day             date PRIMARY KEY,
    egg_farm        numeric(6, 2),               -- 雞蛋產地價
    egg_transport   numeric(6, 2),               -- 雞蛋大運輸價
    broiler_large   numeric(6, 2),               -- 白肉雞 2.0 公斤以上
    broiler_small   numeric(6, 2),               -- 白肉雞 1.75–1.95 公斤
    broiler_retail  numeric(6, 2)                -- 白肉雞門市價（高屏）
);
CREATE TABLE data.hog_trade (
    day           date NOT NULL,
    market        text NOT NULL,
    head          integer NOT NULL,               -- 成交頭數
    price         numeric(7, 2),                  -- 規格豬平均價格（元/公斤）
    weight_kg     numeric(6, 2),                  -- 規格豬平均重量
    PRIMARY KEY (day, market)
);

GRANT SELECT ON data.debris_alert, data.debris_alert_active, ref.flood_sensor, data.flood_current, data.flood_obs,
    data.flood_county_history, ref.ev_station, data.ev_station_live, data.ev_county_history, data.poultry_price,
    data.hog_trade TO dashboard_reader;

-- +goose Down
DROP TABLE data.hog_trade;
DROP TABLE data.poultry_price;
DROP TABLE data.ev_county_history;
DROP TABLE data.ev_station_live;
DROP TABLE ref.ev_station;
DROP TABLE data.flood_county_history;
DROP TABLE data.flood_obs;
DROP TABLE data.flood_current;
DROP TABLE ref.flood_sensor;
DROP VIEW data.debris_alert_active;
DROP TABLE data.debris_alert;
