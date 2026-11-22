-- +goose Up

CREATE TABLE ref.uv_site (
    site_id      serial PRIMARY KEY,
    name         text NOT NULL UNIQUE,
    county_code  char(5) REFERENCES ref.county (code),
    agency       text NOT NULL DEFAULT '',         -- 測站所屬機關（中央氣象署、環境部）
    geom         geometry(Point, 4326),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.uv_obs_history (
    site_id      integer NOT NULL REFERENCES ref.uv_site (site_id),
    observed_at  timestamptz NOT NULL,
    uvi          numeric,                           -- 紫外線指數；來源的 -999（儀器異常）存成 NULL
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, observed_at)
);
CREATE INDEX uv_obs_history_observed_at_idx ON data.uv_obs_history (observed_at);

CREATE TABLE data.uv_obs_current (LIKE data.uv_obs_history INCLUDING DEFAULTS);
ALTER TABLE data.uv_obs_current ADD PRIMARY KEY (site_id);

CREATE TABLE ref.aqi_area (
    area        text PRIMARY KEY,                   -- 來源寫法：北部、竹苗、中部、雲嘉南、高屏、宜蘭、花東、澎湖、金門、馬祖
    name        text NOT NULL,                      -- 顯示名稱：北部空品區…
    sort_order  smallint NOT NULL
);

CREATE TABLE ref.aqi_area_county (
    area         text NOT NULL REFERENCES ref.aqi_area (area),
    county_code  char(5) NOT NULL UNIQUE REFERENCES ref.county (code),
    PRIMARY KEY (area, county_code)
);

INSERT INTO ref.aqi_area (area, name, sort_order) VALUES
    ('北部', '北部空品區', 1), ('竹苗', '竹苗空品區', 2), ('中部', '中部空品區', 3),
    ('雲嘉南', '雲嘉南空品區', 4), ('高屏', '高屏空品區', 5), ('宜蘭', '宜蘭空品區', 6),
    ('花東', '花東空品區', 7), ('澎湖', '澎湖', 8), ('金門', '金門', 9), ('馬祖', '馬祖', 10);

INSERT INTO ref.aqi_area_county (area, county_code) VALUES
    ('北部', '10017'), ('北部', '63000'), ('北部', '65000'), ('北部', '68000'),
    ('竹苗', '10018'), ('竹苗', '10004'), ('竹苗', '10005'),
    ('中部', '66000'), ('中部', '10007'), ('中部', '10008'),
    ('雲嘉南', '10009'), ('雲嘉南', '10020'), ('雲嘉南', '10010'), ('雲嘉南', '67000'),
    ('高屏', '64000'), ('高屏', '10013'),
    ('宜蘭', '10002'),
    ('花東', '10015'), ('花東', '10014'),
    ('澎湖', '10016'), ('金門', '09020'), ('馬祖', '09007');

CREATE TABLE data.aqi_forecast (
    area             text NOT NULL REFERENCES ref.aqi_area (area),
    forecast_date    date NOT NULL,
    published_at     timestamptz NOT NULL,
    aqi              smallint,
    major_pollutant  text,
    minor_pollutant  text,
    minor_aqi        smallint,
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (area, forecast_date, published_at)
);
CREATE INDEX aqi_forecast_published_idx ON data.aqi_forecast (published_at);

CREATE TABLE data.aqi_forecast_text (
    published_at  timestamptz PRIMARY KEY,
    content       text NOT NULL,
    ingested_at   timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT ON ref.uv_site, ref.aqi_area, ref.aqi_area_county,
                data.uv_obs_history, data.uv_obs_current, data.aqi_forecast, data.aqi_forecast_text
    TO dashboard_reader;

-- +goose Down
DROP TABLE data.aqi_forecast_text;
DROP TABLE data.aqi_forecast;
DROP TABLE ref.aqi_area_county;
DROP TABLE ref.aqi_area;
DROP TABLE data.uv_obs_current;
DROP TABLE data.uv_obs_history;
DROP TABLE ref.uv_site;
