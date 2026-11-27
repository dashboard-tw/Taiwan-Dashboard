-- +goose Up

-- 氣象署鄉鎮預報的地點（F-D0047-001 等），代碼是 8 碼的鄉鎮代碼，例如 10002010
CREATE TABLE ref.forecast_town (
    geocode      text PRIMARY KEY,
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    name         text NOT NULL,                     -- 宜蘭市、松山區
    geom         geometry(Point, 4326) NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX forecast_town_county_idx ON ref.forecast_town (county_code);

-- 鄉鎮未來 3 天預報，每 3 小時一個時段
CREATE TABLE data.town_forecast (
    geocode          text NOT NULL REFERENCES ref.forecast_town (geocode),
    starts_at        timestamptz NOT NULL,
    ends_at          timestamptz NOT NULL,
    weather          text,
    weather_code     smallint,
    rain_chance_pct  smallint,                      -- 3 小時降雨機率
    temp_c           smallint,                      -- 時段開始的溫度
    apparent_c       smallint,                      -- 時段開始的體感溫度
    humidity_pct     smallint,
    wind_ms          smallint,
    beaufort         smallint,
    wind_dir         text,
    comfort          text,
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (geocode, starts_at)
);
CREATE INDEX town_forecast_ends_idx ON data.town_forecast (ends_at);

-- 鄉鎮未來 1 週預報，每 12 小時一個時段
CREATE TABLE data.town_forecast_week (
    geocode          text NOT NULL REFERENCES ref.forecast_town (geocode),
    starts_at        timestamptz NOT NULL,
    ends_at          timestamptz NOT NULL,
    weather          text,
    weather_code     smallint,
    rain_chance_pct  smallint,                      -- 12 小時降雨機率，部分時段沒有
    max_temp_c       smallint,
    min_temp_c       smallint,
    max_apparent_c   smallint,
    min_apparent_c   smallint,
    humidity_pct     smallint,
    uvi              smallint,                      -- 紫外線指數，只有白天的時段有
    uv_level         text,                          -- 低量級、中量級、高量級、過量級、危險級
    wind_ms          smallint,
    beaufort         smallint,
    wind_dir         text,
    description      text,                          -- 天氣預報綜合描述
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (geocode, starts_at)
);
CREATE INDEX town_forecast_week_ends_idx ON data.town_forecast_week (ends_at);

-- 潮汐預報的地點（漁港、海水浴場、潛點等）
CREATE TABLE ref.tide_station (
    station_id   text PRIMARY KEY,                  -- 例如 10015050、N00900
    name         text NOT NULL,
    county_code  char(5) REFERENCES ref.county (code),
    geom         geometry(Point, 4326) NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tide_station_county_idx ON ref.tide_station (county_code);

CREATE TABLE data.tide_forecast (
    station_id   text NOT NULL REFERENCES ref.tide_station (station_id),
    at           timestamptz NOT NULL,
    tide         text NOT NULL CHECK (tide IN ('滿潮', '乾潮')),
    height_cm    smallint,                          -- 相對當地平均海平面
    tide_range   text,                              -- 當天潮差：大、中、小
    lunar_date   text,                              -- 農曆日期，例如 2026-11-05
    PRIMARY KEY (station_id, at)
);
CREATE INDEX tide_forecast_at_idx ON data.tide_forecast (at);

-- 各縣市日出日沒、月出月沒（臺北時間）
CREATE TABLE data.sun_moon (
    county_code       char(5) NOT NULL REFERENCES ref.county (code),
    date              date NOT NULL,
    civil_dawn        time,                         -- 民用曙光始
    sunrise           time,
    sunrise_az        smallint,                     -- 日出方位角（度）
    sun_transit       time,                         -- 太陽過中天
    sun_transit_alt   text,                         -- 過中天仰角，例如 44S
    sunset            time,
    sunset_az         smallint,
    civil_dusk        time,                         -- 民用暮光終
    moonrise          time,                         -- 當天沒有月出時是空值
    moonrise_az       smallint,
    moon_transit      time,
    moon_transit_alt  text,
    moonset           time,
    moonset_az        smallint,
    PRIMARY KEY (county_code, date)
);

-- 海嘯消息與警報
CREATE TABLE data.tsunami (
    tsunami_no    integer NOT NULL,                 -- 例如 115007
    report_seq    smallint NOT NULL,                -- 第幾報
    issued_at     timestamptz NOT NULL,
    valid_until   timestamptz,
    report_type   text,                             -- 海嘯消息、海嘯警報
    report_color  text,                             -- 綠色、黃色、橘色、紅色
    content       text,
    report_url    text,
    origin_time   timestamptz,                      -- 地震發生時間
    source        text,                             -- 通報單位
    location      text,                             -- 震央位置
    geom          geometry(Point, 4326),
    magnitude     numeric,
    depth_km      numeric,
    ingested_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tsunami_no, report_seq)
);
CREATE INDEX tsunami_issued_idx ON data.tsunami (issued_at);

GRANT SELECT ON ref.forecast_town, data.town_forecast, data.town_forecast_week, ref.tide_station,
    data.tide_forecast, data.sun_moon, data.tsunami TO dashboard_reader;

-- +goose Down
DROP TABLE data.tsunami;
DROP TABLE data.sun_moon;
DROP TABLE data.tide_forecast;
DROP TABLE ref.tide_station;
DROP TABLE data.town_forecast_week;
DROP TABLE data.town_forecast;
DROP TABLE ref.forecast_town;
