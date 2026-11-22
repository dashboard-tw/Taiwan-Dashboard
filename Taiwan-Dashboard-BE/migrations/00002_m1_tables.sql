-- +goose Up

CREATE TABLE ref.county (
    code        char(5)  PRIMARY KEY,               -- 內政部縣市代碼，例如 63000
    name        text     NOT NULL UNIQUE,           -- 臺北市
    short_name  text     NOT NULL,                  -- 臺北（新竹縣／市、嘉義縣／市保留全名）
    name_en     text     NOT NULL,                  -- Taipei City
    aliases     text[]   NOT NULL DEFAULT '{}',     -- 台北市 等來源不一致的寫法
    boundary    geometry(MultiPolygon, 4326) NOT NULL,
    center      geometry(Point, 4326) NOT NULL,     -- 地圖標籤位置
    sort_order  smallint NOT NULL
);
CREATE INDEX county_boundary_gix ON ref.county USING gist (boundary);

CREATE TABLE ref.aqi_site (
    site_id     integer PRIMARY KEY,                -- 環境部測站編號
    name        text NOT NULL,
    county_code char(5) REFERENCES ref.county (code),
    geom        geometry(Point, 4326),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ref.reservoir (
    reservoir_id        varchar(5) PRIMARY KEY,     -- 水利署代碼，前兩碼為區域
    name                text NOT NULL,
    county_code         char(5) REFERENCES ref.county (code),
    effective_capacity  numeric,                    -- 有效容量（萬立方公尺）
    capacity_updated_on date,                       -- 有效容量最後一次變動的日期
    geom                geometry(Point, 4326),      -- 人工標註（後台）
    is_major            boolean NOT NULL DEFAULT false,
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ref.power_unit (
    unit_id     serial PRIMARY KEY,
    unit_name   text NOT NULL,                      -- 大潭CC#1
    plant_name  text,                               -- 大潭
    fuel_type   text NOT NULL,                      -- gas、coal、solar…
    is_private  boolean NOT NULL DEFAULT false,     -- 民營電廠
    county_code char(5) REFERENCES ref.county (code), -- 之後人工補上
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (unit_name, fuel_type)
);

CREATE TABLE data.aqi_obs_history (
    site_id               integer NOT NULL REFERENCES ref.aqi_site (site_id),
    observed_at           timestamptz NOT NULL,
    aqi                   smallint,
    status                text,
    pollutant             text,
    pm25_ug_m3            numeric,
    pm25_avg_ug_m3        numeric,
    pm10_ug_m3            numeric,
    pm10_avg_ug_m3        numeric,
    o3_ppb                numeric,
    o3_8hr_ppb            numeric,
    co_ppm                numeric,
    co_8hr_ppm            numeric,
    so2_ppb               numeric,
    so2_avg_ppb           numeric,
    no2_ppb               numeric,
    nox_ppb               numeric,
    no_ppb                numeric,
    wind_speed_m_s        numeric,
    wind_dir_deg          numeric,
    ingested_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, observed_at)
);
CREATE INDEX aqi_obs_history_observed_at_idx ON data.aqi_obs_history (observed_at);

CREATE TABLE data.aqi_obs_current (LIKE data.aqi_obs_history INCLUDING DEFAULTS);
ALTER TABLE data.aqi_obs_current ADD PRIMARY KEY (site_id);

CREATE TABLE data.reservoir_obs_history (
    reservoir_id       varchar(5) NOT NULL REFERENCES ref.reservoir (reservoir_id),
    observed_at        timestamptz NOT NULL,
    water_level_m      numeric,
    effective_storage  numeric,                     -- 有效蓄水量（萬立方公尺）
    storage_pct        numeric,                     -- 寫入時依 ref.reservoir.effective_capacity 計算
    inflow_cms         numeric,
    total_outflow_cms  numeric,
    rainfall_mm        numeric,
    status_type        smallint,
    ingested_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (reservoir_id, observed_at)
);
CREATE INDEX reservoir_obs_history_observed_at_idx ON data.reservoir_obs_history (observed_at);

CREATE TABLE data.reservoir_obs_current (LIKE data.reservoir_obs_history INCLUDING DEFAULTS);
ALTER TABLE data.reservoir_obs_current ADD PRIMARY KEY (reservoir_id);

CREATE TABLE data.power_gen_history (
    unit_id      integer NOT NULL REFERENCES ref.power_unit (unit_id),
    observed_at  timestamptz NOT NULL,
    capacity_mw  numeric,
    net_gen_mw   numeric,
    remark       text,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (unit_id, observed_at)
);
CREATE INDEX power_gen_history_observed_at_idx ON data.power_gen_history (observed_at);

CREATE TABLE data.power_gen_by_fuel_history (
    observed_at  timestamptz NOT NULL,
    fuel_type    text NOT NULL,
    net_gen_mw   numeric NOT NULL,
    share_pct    numeric,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (observed_at, fuel_type)
);

CREATE TABLE data.power_gen_by_fuel_current (LIKE data.power_gen_by_fuel_history INCLUDING DEFAULTS);
ALTER TABLE data.power_gen_by_fuel_current ADD PRIMARY KEY (fuel_type);

CREATE TABLE data.power_supply_daily (
    date                  date PRIMARY KEY,
    net_peak_capacity_mw  numeric,                  -- 原始單位萬瓩 × 10
    peak_load_mw          numeric,
    reserve_mw            numeric,
    reserve_pct           numeric,
    industrial_gwh        numeric,                  -- 原始單位百萬度 = GWh
    residential_gwh       numeric,
    ingested_at           timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.power_supply_forecast (
    forecast_date         date NOT NULL,
    published_on          date NOT NULL,
    net_peak_capacity_mw  numeric,
    peak_load_mw          numeric,
    reserve_mw            numeric,
    reserve_pct           numeric,
    ingested_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (forecast_date, published_on)
);

CREATE TABLE app.dataset (
    id                text PRIMARY KEY,             -- aqi_obs、reservoir_obs、power_gen…
    name              text NOT NULL,
    source_agency     text NOT NULL,
    source_url        text NOT NULL,
    license           text NOT NULL,
    update_freq       interval NOT NULL,            -- 預期的更新頻率
    stale_after       interval NOT NULL,            -- 資料超過這個時間沒更新就算延遲
    last_observed_at  timestamptz,                  -- 資料本身的最新時間
    last_ingested_at  timestamptz,                  -- 最後一次成功抓取的時間
    status            text NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'ok', 'stale', 'error')),
    last_error        text,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.pipeline_run (
    id             bigserial PRIMARY KEY,
    dataset_id     text NOT NULL REFERENCES app.dataset (id),
    started_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz,
    status         text NOT NULL DEFAULT 'running'
                   CHECK (status IN ('running', 'success', 'partial', 'failed')),
    rows_fetched   integer NOT NULL DEFAULT 0,
    rows_inserted  integer NOT NULL DEFAULT 0,
    warnings       jsonb NOT NULL DEFAULT '[]',
    error          text
);
CREATE INDEX pipeline_run_dataset_started_idx ON app.pipeline_run (dataset_id, started_at DESC);

-- +goose Down
DROP TABLE app.pipeline_run;
DROP TABLE app.dataset;
DROP TABLE data.power_supply_forecast;
DROP TABLE data.power_supply_daily;
DROP TABLE data.power_gen_by_fuel_current;
DROP TABLE data.power_gen_by_fuel_history;
DROP TABLE data.power_gen_history;
DROP TABLE data.reservoir_obs_current;
DROP TABLE data.reservoir_obs_history;
DROP TABLE data.aqi_obs_current;
DROP TABLE data.aqi_obs_history;
DROP TABLE ref.power_unit;
DROP TABLE ref.reservoir;
DROP TABLE ref.aqi_site;
DROP TABLE ref.county;
