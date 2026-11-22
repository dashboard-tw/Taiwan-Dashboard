-- +goose Up

CREATE TABLE data.rail_alert (
    operator       text NOT NULL CHECK (operator IN ('TRA', 'THSR')),
    alert_id       text NOT NULL,
    title          text NOT NULL,
    description    text,
    reason         text,
    status         smallint,
    level          smallint,
    lines          text[] NOT NULL DEFAULT '{}',    -- 影響的路線，例如平溪線
    stations       text[] NOT NULL DEFAULT '{}',    -- 影響的車站名稱
    starts_at      timestamptz,
    ends_at        timestamptz,
    published_at   timestamptz,
    updated_at     timestamptz,
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (operator, alert_id)
);

CREATE VIEW data.rail_alert_active AS
SELECT *
FROM data.rail_alert
WHERE last_seen_at >= now() - interval '30 minutes'
  AND (ends_at IS NULL OR ends_at > now());

CREATE TABLE ref.tra_station (
    station_id     text PRIMARY KEY,
    name           text NOT NULL,
    county_code    char(5) REFERENCES ref.county (code),
    address        text,
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.tra_train_live (
    train_no        text PRIMARY KEY,
    train_type      text,                           -- 車種，例如區間、自強
    station_id      text,
    station_name    text,
    station_status  smallint,                       -- 0 進站中、1 在站上、2 已離站
    delay_min       smallint NOT NULL,
    updated_at      timestamptz
);

CREATE TABLE data.tra_delay_history (
    observed_at    timestamptz PRIMARY KEY,
    running        integer NOT NULL,
    delayed        integer NOT NULL,
    avg_delay_min  numeric(6, 2),
    max_delay_min  integer NOT NULL
);

CREATE TABLE ref.bike_station (
    station_uid    text PRIMARY KEY,
    city           text NOT NULL,                   -- TDX 的縣市名稱，例如 Taipei
    county_code    char(5) NOT NULL REFERENCES ref.county (code),
    name           text NOT NULL,
    address        text,
    capacity       integer,
    service_type   smallint,
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bike_station_county_idx ON ref.bike_station (county_code);

CREATE TABLE data.bike_availability_current (
    station_uid  text PRIMARY KEY REFERENCES ref.bike_station (station_uid),
    status       smallint NOT NULL,
    bikes        integer NOT NULL,                  -- 可借車輛
    docks        integer NOT NULL,                  -- 可還空位
    ebikes       integer NOT NULL DEFAULT 0,        -- 其中電輔車
    updated_at   timestamptz,                       -- 來源的更新時間
    fetched_at   timestamptz NOT NULL               -- 這次抓取的時間（同一次更新的各站相同）
);

CREATE TABLE data.bike_county_history (
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    observed_at  timestamptz NOT NULL,
    stations     integer NOT NULL,
    bikes        integer NOT NULL,
    docks        integer NOT NULL,
    empty_stations  integer NOT NULL,           -- 沒有車可借的站
    full_stations   integer NOT NULL,           -- 沒有空位可還的站
    PRIMARY KEY (county_code, observed_at)
);
CREATE INDEX bike_county_history_observed_idx ON data.bike_county_history (observed_at);

GRANT SELECT ON data.rail_alert, data.rail_alert_active, ref.tra_station, data.tra_train_live, data.tra_delay_history,
    ref.bike_station, data.bike_availability_current, data.bike_county_history TO dashboard_reader;

-- +goose Down
DROP TABLE data.bike_county_history;
DROP TABLE data.bike_availability_current;
DROP TABLE ref.bike_station;
DROP TABLE data.tra_delay_history;
DROP TABLE data.tra_train_live;
DROP TABLE ref.tra_station;
DROP VIEW data.rail_alert_active;
DROP TABLE data.rail_alert;
