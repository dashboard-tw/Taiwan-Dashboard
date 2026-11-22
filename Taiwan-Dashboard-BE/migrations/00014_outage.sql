-- +goose Up

ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage']);

CREATE TABLE data.water_outage (
    source               text NOT NULL CHECK (source IN ('twc', 'taipei')), -- 台灣自來水、臺北自來水
    case_id              text NOT NULL,             -- 來源的案件編號
    county_code          char(5) REFERENCES ref.county (code),
    district             text,                      -- 影響行政區（可能有多個，以逗號分隔）
    area                 text NOT NULL,             -- 停水地區
    reason               text,
    kind                 text,                      -- 計畫性、非計畫性（臺北自來水沒有這個欄位）
    households           integer,                   -- 影響戶數（臺北自來水沒有這個欄位）
    starts_at            timestamptz NOT NULL,
    ends_at              timestamptz NOT NULL,      -- 預計恢復供水
    daily_from           time,                      -- 臺北自來水的公告是「期間內每天幾點到幾點」
    daily_to             time,
    low_pressure_area    text,                      -- 降壓地區
    low_pressure_reason  text,
    url                  text,
    published_on         date,
    first_seen_at        timestamptz NOT NULL DEFAULT now(),
    last_seen_at         timestamptz NOT NULL DEFAULT now(), -- 最後一次在來源出現
    updated_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, case_id),
    CHECK (ends_at >= starts_at)
);
CREATE INDEX water_outage_period_idx ON data.water_outage (ends_at, starts_at);

CREATE TABLE data.power_outage_history (
    observed_at     timestamptz NOT NULL,           -- 台電的資料更新時間，沒有時用抓取時間
    seq             smallint NOT NULL,              -- 同一次資料裡的順序
    county_code     char(5) REFERENCES ref.county (code),
    district        text,                           -- 行政區
    area            text,                           -- 停電區域
    info            text,                           -- 停電資訊
    households      integer,                        -- 影響戶數
    expected_restore text,                          -- 預計修復時間（來源的文字）
    ingested_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (observed_at, seq)
);

CREATE TABLE data.power_outage_current (LIKE data.power_outage_history INCLUDING DEFAULTS);
ALTER TABLE data.power_outage_current ADD PRIMARY KEY (seq);

GRANT SELECT ON data.water_outage, data.power_outage_history, data.power_outage_current TO dashboard_reader;

-- +goose Down
DROP TABLE data.power_outage_current;
DROP TABLE data.power_outage_history;
DROP TABLE data.water_outage;
SELECT set_config('app.rls_bypass', 'on', true);
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic = 'outage');
DELETE FROM app.dashboard WHERE slug = 'outage';
DELETE FROM app.component WHERE topic = 'outage';
UPDATE app.api_survey SET topics = array_remove(topics, 'outage') WHERE 'outage' = ANY (topics);
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power'));
