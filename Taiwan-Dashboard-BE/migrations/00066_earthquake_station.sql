-- +goose Up

ALTER TABLE data.earthquake
    ADD COLUMN shakemap_url  text,                  -- 震度分布圖
    ADD COLUMN report_type   text,                  -- 地震報告
    ADD COLUMN report_remark text;                  -- 報告附註

-- 各測站的震度與強地動（E-A0015-001、E-A0016-001 的 EqStation）
CREATE TABLE data.earthquake_station (
    report_id         text NOT NULL REFERENCES data.earthquake (report_id) ON DELETE CASCADE,
    station_id        text NOT NULL,
    name              text NOT NULL,
    county_code       char(5) REFERENCES ref.county (code),
    area              text,                         -- 來源的地區名稱，例如臺東縣地區
    geom              geometry(Point, 4326),
    intensity         text,                         -- 4級、5弱
    intensity_rank    smallint,                     -- 同 data.earthquake.max_intensity_rank
    pga_gal           numeric,                      -- 震度計算用的最大加速度（gal）
    pga_ew_gal        numeric,
    pga_ns_gal        numeric,
    pga_v_gal         numeric,
    pgv_kine          numeric,                      -- 震度計算用的最大速度（kine，公分/秒）
    pgv_ew_kine       numeric,
    pgv_ns_kine       numeric,
    pgv_v_kine        numeric,
    distance_km       numeric,                      -- 離震央距離
    back_azimuth      numeric,                      -- 測站看震央的方位角
    wave_url          text,                         -- 強地動波形圖
    info_status       text,                         -- observe 等
    PRIMARY KEY (report_id, station_id)
);
CREATE INDEX earthquake_station_county_idx ON data.earthquake_station (county_code);

GRANT SELECT ON data.earthquake_station TO dashboard_reader;

-- +goose Down
DROP TABLE data.earthquake_station;
ALTER TABLE data.earthquake DROP COLUMN shakemap_url, DROP COLUMN report_type, DROP COLUMN report_remark;
