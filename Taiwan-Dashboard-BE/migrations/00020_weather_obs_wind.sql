-- +goose Up

ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning']);

ALTER TABLE app.component_map_layer DROP CONSTRAINT component_map_layer_layer_type_check;
ALTER TABLE app.component_map_layer ADD CONSTRAINT component_map_layer_layer_type_check
    CHECK (layer_type IN ('circle', 'symbol', 'fill', 'line', 'heatmap', 'raster', 'wind'));
ALTER TABLE data.weather_image DROP CONSTRAINT weather_image_product_check;
ALTER TABLE data.weather_image ADD CONSTRAINT weather_image_product_check
    CHECK (product IN ('radar', 'satellite', 'radar_layer', 'cloud_layer', 'wind_layer'));

CREATE TABLE ref.weather_station (
    station_id   text PRIMARY KEY,                  -- 氣象署站號：46 開頭是有人的氣象站，其他是自動站
    name         text NOT NULL,
    kind         text NOT NULL CHECK (kind IN ('manned', 'auto')),
    county_code  char(5) REFERENCES ref.county (code),
    town         text,
    altitude_m   numeric,
    geom         geometry(Point, 4326),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.weather_obs_history (
    station_id    text NOT NULL REFERENCES ref.weather_station (station_id),
    observed_at   timestamptz NOT NULL,
    weather       text,                             -- 天氣現象，例如「晴」「多雲有雨」（只有部分測站有）
    temp_c        numeric,
    humidity_pct  numeric,
    pressure_hpa  numeric,
    wind_dir_deg  numeric,                          -- 風從哪裡吹來，0 是北風、90 是東風
    wind_ms       numeric,                          -- 10 分鐘平均風速（公尺/秒）
    gust_ms       numeric,                          -- 本日最大陣風
    high_c        numeric,                          -- 本日最高溫
    low_c         numeric,                          -- 本日最低溫
    ingested_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (station_id, observed_at)
);
CREATE INDEX weather_obs_history_observed_at_idx ON data.weather_obs_history (observed_at);

CREATE TABLE data.weather_obs_current (LIKE data.weather_obs_history INCLUDING DEFAULTS);
ALTER TABLE data.weather_obs_current ADD PRIMARY KEY (station_id);

CREATE TABLE data.county_forecast (
    county_code      char(5) NOT NULL REFERENCES ref.county (code),
    starts_at        timestamptz NOT NULL,
    ends_at          timestamptz NOT NULL,
    weather          text NOT NULL,                 -- 天氣現象，例如「多雲時晴」
    weather_code     smallint,                      -- 氣象署的天氣現象代碼
    rain_chance_pct  smallint,                      -- 降雨機率
    min_temp_c       smallint,
    max_temp_c       smallint,
    comfort          text,                          -- 舒適度，例如「舒適至悶熱」
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (county_code, starts_at)
);
CREATE INDEX county_forecast_starts_at_idx ON data.county_forecast (starts_at);

GRANT SELECT ON ref.weather_station, data.weather_obs_history, data.weather_obs_current, data.county_forecast
    TO dashboard_reader;

-- +goose Down
DROP TABLE data.county_forecast;
DROP TABLE data.weather_obs_current;
DROP TABLE data.weather_obs_history;
DROP TABLE ref.weather_station;
DELETE FROM data.weather_image WHERE product = 'wind_layer';
ALTER TABLE data.weather_image DROP CONSTRAINT weather_image_product_check;
ALTER TABLE data.weather_image ADD CONSTRAINT weather_image_product_check
    CHECK (product IN ('radar', 'satellite', 'radar_layer', 'cloud_layer'));
DELETE FROM app.component_map_layer WHERE layer_type = 'wind';
ALTER TABLE app.component_map_layer DROP CONSTRAINT component_map_layer_layer_type_check;
ALTER TABLE app.component_map_layer ADD CONSTRAINT component_map_layer_layer_type_check
    CHECK (layer_type IN ('circle', 'symbol', 'fill', 'line', 'heatmap', 'raster'));
UPDATE app.component SET topic = 'weather' WHERE topic = 'warning';
UPDATE app.api_survey SET topics = ARRAY(SELECT DISTINCT unnest(array_replace(topics, 'warning', 'weather')))
 WHERE 'warning' = ANY (topics);
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather'));
