-- +goose Up

-- 新頁面：天氣預報、健康氣象、海洋與天文
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing', 'disease', 'aviation', 'economy', 'animal', 'forecast', 'health', 'ocean'));

-- 低溫特報、高溫資訊、濃霧特報（W-C0033-004、005、010），每個資料集只有最近一次
CREATE TABLE data.weather_advisory (
    event        text NOT NULL CHECK (event IN ('低溫', '高溫', '濃霧')),
    effective    timestamptz NOT NULL,
    onset        timestamptz,
    expires      timestamptz,
    headline     text,                              -- 低溫特報、高溫資訊、濃霧特報
    level        text,                              -- 低溫黃色燈號、橙色燈號等，沒有時用燈號顏色
    color        text,                              -- 黃色、橙色、紅色
    description  text,
    instruction  text,
    web          text,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event, effective)
);

CREATE TABLE data.weather_advisory_area (
    event        text NOT NULL,
    effective    timestamptz NOT NULL,
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    town         text NOT NULL DEFAULT '',          -- 空字串代表整個縣市
    PRIMARY KEY (event, effective, county_code, town),
    FOREIGN KEY (event, effective) REFERENCES data.weather_advisory (event, effective) ON DELETE CASCADE
);

-- 健康氣象：熱傷害（逐 3 小時）、冷傷害與溫差提醒（逐日），未來 5 天
CREATE TABLE data.town_health (
    geocode      text NOT NULL REFERENCES ref.forecast_town (geocode),
    kind         text NOT NULL CHECK (kind IN ('heat', 'cold', 'temp_diff')),
    starts_at    timestamptz NOT NULL,
    ends_at      timestamptz NOT NULL,
    index_value  smallint,                          -- 熱傷害指數、冷傷害指數；溫差提醒沒有
    max_temp_c   smallint,                          -- 溫差提醒的最高溫與最低溫
    min_temp_c   smallint,
    warning      text,                              -- 注意、警戒、危險、高危險，沒有提醒時是空值
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (geocode, kind, starts_at)
);
CREATE INDEX town_health_ends_idx ON data.town_health (kind, ends_at);

-- 有人氣象站各月平年值（C-B0027-001）
CREATE TABLE data.climate_normal (
    station_id    text NOT NULL,
    station_name  text NOT NULL,
    month         smallint NOT NULL CHECK (month BETWEEN 1 AND 12),
    temp_mean_c   numeric,
    temp_max_c    numeric,                          -- 平均最高溫
    temp_min_c    numeric,                          -- 平均最低溫
    rain_mm       numeric,                          -- 月雨量
    rain_days     numeric,                          -- 雨量 0.1 毫米以上的日數
    start_year    smallint,
    end_year      smallint,
    PRIMARY KEY (station_id, month)
);

-- 有人氣象站今年每日雨量（C-B0025-001），T（微量）記為 0
CREATE TABLE data.rain_daily (
    station_id    text NOT NULL,
    station_name  text NOT NULL,
    date          date NOT NULL,
    rain_mm       numeric,
    PRIMARY KEY (station_id, date)
);

GRANT SELECT ON data.weather_advisory, data.weather_advisory_area, data.town_health, data.climate_normal,
    data.rain_daily TO dashboard_reader;

-- +goose Down
DROP TABLE data.rain_daily;
DROP TABLE data.climate_normal;
DROP TABLE data.town_health;
DROP TABLE data.weather_advisory_area;
DROP TABLE data.weather_advisory;
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing', 'disease', 'aviation', 'economy', 'animal'));
