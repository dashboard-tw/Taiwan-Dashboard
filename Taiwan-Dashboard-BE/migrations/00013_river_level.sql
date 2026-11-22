-- +goose Up

CREATE TABLE ref.river_station (
    station_id   varchar(10) PRIMARY KEY,            -- 水利署站號，例如 1140H006
    name         text NOT NULL,                     -- 去掉「(即時)」後綴的站名
    river_name   text,                              -- 所屬河川
    basin_code   text,                              -- 所屬流域代碼
    county_code  char(5) REFERENCES ref.county (code),
    address      text,
    geom         geometry(Point, 4326),             -- 由 TWD97 座標轉換；與縣市不符時不存
    alert1_m     numeric,                           -- 一級警戒水位（公尺）
    alert2_m     numeric,                           -- 二級警戒水位
    alert3_m     numeric,                           -- 三級警戒水位
    is_active    boolean NOT NULL DEFAULT true,     -- 站況為「現存」
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX river_station_county_idx ON ref.river_station (county_code);

CREATE TABLE data.river_level_history (
    station_id     varchar(10) NOT NULL REFERENCES ref.river_station (station_id),
    observed_at    timestamptz NOT NULL,
    water_level_m  numeric,                         -- 水位（公尺）
    ingested_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (station_id, observed_at)
);
CREATE INDEX river_level_history_observed_at_idx ON data.river_level_history (observed_at);

CREATE TABLE data.river_level_current (LIKE data.river_level_history INCLUDING DEFAULTS);
ALTER TABLE data.river_level_current ADD PRIMARY KEY (station_id);

GRANT SELECT ON ref.river_station, data.river_level_history, data.river_level_current TO dashboard_reader;

-- +goose Down
DROP TABLE data.river_level_current;
DROP TABLE data.river_level_history;
DROP TABLE ref.river_station;
