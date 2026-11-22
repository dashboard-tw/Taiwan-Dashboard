-- +goose Up

ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather']);

CREATE TABLE data.typhoon (
    typhoon_id     text PRIMARY KEY,                -- 例如 2026-33
    year           smallint NOT NULL,
    td_no          smallint NOT NULL,               -- 氣象署熱帶性低氣壓編號
    ty_no          smallint,                        -- 氣象署颱風編號（第幾號颱風）
    name_en        text,                            -- 國際命名，例如 CHAMPI
    name_zh        text,                            -- 中文名稱，例如 薔琵
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now(), -- 最後一次出現在「活動中的熱帶氣旋」清單
    UNIQUE (year, td_no)
);

CREATE TABLE data.typhoon_fix (
    typhoon_id       text NOT NULL REFERENCES data.typhoon (typhoon_id),
    kind             text NOT NULL CHECK (kind IN ('analysis', 'forecast')),
    issued_at        timestamptz NOT NULL,          -- 分析：該點的時間；預報：預報的起始時間
    forecast_hour    smallint NOT NULL DEFAULT 0,   -- 預報幾小時後；分析為 0
    valid_at         timestamptz NOT NULL,          -- 位置的時間（issued_at + forecast_hour）
    geom             geometry(Point, 4326) NOT NULL,
    pressure_hpa     smallint,
    max_wind_ms      smallint,                      -- 近中心最大風速（公尺/秒）
    max_gust_ms      smallint,
    moving_speed_kmh smallint,
    moving_dir       text,                          -- 16 方位，例如 NW
    radius7_km       smallint,                      -- 七級風暴風半徑
    radius10_km      smallint,                      -- 十級風暴風半徑
    radius70_km      smallint,                      -- 70% 機率半徑（只有預報）
    note             text,                          -- 例如「變性為溫帶氣旋」
    PRIMARY KEY (typhoon_id, kind, issued_at, forecast_hour)
);
CREATE INDEX typhoon_fix_valid_idx ON data.typhoon_fix (valid_at);

CREATE TABLE data.typhoon_warning (
    effective_at   timestamptz NOT NULL,            -- 發布時間
    ty_no          smallint NOT NULL,
    warning_no     smallint,                        -- 第幾報
    name_zh        text,
    name_en        text,
    alert          text NOT NULL,                   -- 海上颱風警報、海上陸上颱風警報、解除颱風警報…
    expires_at     timestamptz,
    sections       jsonb NOT NULL,                  -- [{title, value}]：命名與位置、強度與半徑、警戒區域及事項…
    areas          text[] NOT NULL DEFAULT '{}',    -- 警戒的海面與縣市
    url            text,
    ingested_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (ty_no, effective_at)
);

CREATE TABLE data.weather_warning (
    county_code    char(5) NOT NULL REFERENCES ref.county (code),
    phenomena      text NOT NULL,                   -- 海上陸上颱風、陸上強風、豪雨、低溫…
    significance   text NOT NULL,                   -- 警報、特報
    starts_at      timestamptz NOT NULL,
    ends_at        timestamptz,
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now(), -- 最後一次在來源出現；元件只看最近一小時內還在的
    PRIMARY KEY (county_code, phenomena, significance, starts_at)
);
CREATE INDEX weather_warning_seen_idx ON data.weather_warning (last_seen_at);

CREATE TABLE ref.rain_station (
    station_id   text PRIMARY KEY,                  -- 氣象署站號，例如 C1I230
    name         text NOT NULL,
    county_code  char(5) REFERENCES ref.county (code),
    town         text,
    altitude_m   numeric,
    maintainer   text,                              -- 維護單位：中央氣象署、水利署…
    geom         geometry(Point, 4326),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.rain_obs_history (
    station_id      text NOT NULL REFERENCES ref.rain_station (station_id),
    observed_at     timestamptz NOT NULL,
    today_mm        numeric,                        -- 本日累積（從 0 時起）
    past10min_mm    numeric,
    past1hr_mm      numeric,
    past3hr_mm      numeric,
    past6hr_mm      numeric,
    past12hr_mm     numeric,
    past24hr_mm     numeric,
    past2days_mm    numeric,
    past3days_mm    numeric,
    ingested_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (station_id, observed_at)
);
CREATE INDEX rain_obs_history_observed_at_idx ON data.rain_obs_history (observed_at);

CREATE TABLE data.rain_obs_current (LIKE data.rain_obs_history INCLUDING DEFAULTS);
ALTER TABLE data.rain_obs_current ADD PRIMARY KEY (station_id);

GRANT SELECT ON data.typhoon, data.typhoon_fix, data.typhoon_warning, data.weather_warning,
                ref.rain_station, data.rain_obs_history, data.rain_obs_current
    TO dashboard_reader;

-- +goose Down
DROP TABLE data.rain_obs_current;
DROP TABLE data.rain_obs_history;
DROP TABLE ref.rain_station;
DROP TABLE data.weather_warning;
DROP TABLE data.typhoon_warning;
DROP TABLE data.typhoon_fix;
DROP TABLE data.typhoon;
SELECT set_config('app.rls_bypass', 'on', true);
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic = 'weather');
DELETE FROM app.dashboard WHERE slug = 'weather';
DELETE FROM app.component WHERE topic = 'weather';
UPDATE app.api_survey SET topics = array_remove(topics, 'weather') WHERE 'weather' = ANY (topics);
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake'));
