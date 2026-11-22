-- +goose Up

ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources']);

CREATE TABLE ref.medical_facility (
    facility_id    text PRIMARY KEY,                -- 健保署醫事機構代碼
    name           text NOT NULL,
    level          text NOT NULL CHECK (level IN ('medical_center', 'regional', 'district', 'clinic', 'pharmacy')),
    kind           text,                            -- 醫事機構種類，例如綜合醫院、專科診所、牙醫一般診所、藥師自營
    county_code    char(5) REFERENCES ref.county (code),
    address        text,
    phone          text,
    departments    text,                            -- 診療科別
    services       text,                            -- 服務項目
    contract_end   date,                            -- 終止合約或歇業日期
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now() -- 不在最新名單上的機構，元件就不算
);
CREATE INDEX medical_facility_county_idx ON ref.medical_facility (county_code, level);

CREATE TABLE ref.ltc_facility (
    facility_id    text PRIMARY KEY,                -- 機構代碼
    name           text NOT NULL,
    abc            char(1) CHECK (abc IN ('A', 'B', 'C')),
    county_code    char(5) REFERENCES ref.county (code),
    town_code      text,
    address        text,
    services       text,                            -- 特約服務項目，以頓號分隔
    beds           integer,                         -- 開放床數（住宿式機構）
    residents      integer,
    contract_end   date,
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ltc_facility_county_idx ON ref.ltc_facility (county_code);

CREATE TABLE ref.shelter (
    shelter_id     text PRIMARY KEY,                -- EMIC 的收容所代碼
    code           text,
    name           text NOT NULL,
    county_code    char(5) REFERENCES ref.county (code),
    town           text,
    address        text,
    disaster_types text,                            -- 適用災害，例如「水災,震災,土石流」
    indoor         boolean,
    outdoor        boolean,
    for_weaker     boolean,                         -- 適合避難弱者安置
    is_open        boolean NOT NULL DEFAULT false,
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX shelter_county_idx ON ref.shelter (county_code);

CREATE TABLE data.shelter_report (
    shelter_id      text NOT NULL,                  -- 對得到 ref.shelter 時可以畫在地圖上
    opened_at       timestamptz NOT NULL,
    closed_at       timestamptz,
    county_code     char(5) REFERENCES ref.county (code),
    town            text,
    name            text NOT NULL,
    refugees        integer,                        -- 目前收容人數
    refugees_total  integer,                        -- 累計收容人數
    capacity        integer,                        -- 場所可收容人數
    project         text,                           -- 災害專案代號
    reported_at     timestamptz,                    -- 通報時間
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (shelter_id, opened_at)
);
CREATE INDEX shelter_report_seen_idx ON data.shelter_report (last_seen_at);

GRANT SELECT ON ref.medical_facility, ref.ltc_facility, ref.shelter, data.shelter_report TO dashboard_reader;

-- +goose Down
DROP TABLE data.shelter_report;
DROP TABLE ref.shelter;
DROP TABLE ref.ltc_facility;
DROP TABLE ref.medical_facility;
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic = 'resources');
DELETE FROM app.component WHERE topic = 'resources';
UPDATE app.api_survey SET topics = array_remove(topics, 'resources') WHERE 'resources' = ANY (topics);
DELETE FROM app.api_survey WHERE cardinality(topics) = 0;
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic'));
