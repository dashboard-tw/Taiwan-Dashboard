-- +goose Up

CREATE TABLE data.power_region_history (
    observed_at  timestamptz NOT NULL,
    region       text NOT NULL CHECK (region IN ('北部', '中部', '南部', '東部')),
    gen_mw       numeric,                           -- 發電（來源單位萬瓩，×10 換成 MW）
    load_mw      numeric,                           -- 用電
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (observed_at, region)
);

CREATE TABLE data.power_load_history (
    observed_at            timestamptz PRIMARY KEY, -- 台電的發布時間（每 10 分鐘）
    load_mw                numeric,                 -- 目前用電量
    util_pct               numeric,                 -- 使用率（目前用電量 ÷ 今日最大供電能力）
    fore_supply_mw         numeric,                 -- 今日預估最大供電能力
    fore_peak_load_mw      numeric,                 -- 今日預估尖峰負載
    fore_reserve_mw        numeric,                 -- 今日預估尖峰備轉容量
    fore_reserve_pct       numeric,                 -- 今日預估尖峰備轉容量率
    fore_indicator         text,                    -- 供電燈號：G 綠、Y 黃、O 橘、R 紅、B 黑
    fore_peak_hours        text,                    -- 預估尖峰時段，例如 13:00-16:00
    ingested_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.power_peak_daily (
    date          date PRIMARY KEY,
    supply_mw     numeric,                          -- 最大供電能力
    peak_load_mw  numeric,                          -- 尖峰負載
    reserve_mw    numeric,                          -- 尖峰備轉容量
    reserve_pct   numeric,                          -- 尖峰備轉容量率
    indicator     text,                             -- 供電燈號
    ingested_at   timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT ON data.power_region_history, data.power_load_history, data.power_peak_daily
    TO dashboard_reader;

-- +goose Down
DROP TABLE data.power_peak_daily;
DROP TABLE data.power_load_history;
DROP TABLE data.power_region_history;
