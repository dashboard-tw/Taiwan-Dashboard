-- +goose Up
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain']);

CREATE TABLE data.vital_month (
    month        date NOT NULL,                     -- 該月 1 日
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    births       integer NOT NULL,
    deaths       integer NOT NULL,
    marriages    integer NOT NULL,
    divorces     integer NOT NULL,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (month, county_code)
);

CREATE TABLE ref.childcare_center (
    facility_id    text PRIMARY KEY,                -- 社福機構代碼
    name           text NOT NULL,
    county_code    char(5) REFERENCES ref.county (code),
    address        text,
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX childcare_center_county_idx ON ref.childcare_center (county_code);

CREATE TABLE data.power_sales_month (
    month        date NOT NULL,
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    sector       text NOT NULL CHECK (sector IN ('residential', 'service', 'agriculture', 'industry')),
    kwh          bigint NOT NULL,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (month, county_code, sector)
);

CREATE TABLE data.waste_county (
    period_type    text NOT NULL CHECK (period_type IN ('year', 'month')),
    period         date NOT NULL,                   -- 該年 1 月 1 日或該月 1 日
    county_code    char(5) NOT NULL REFERENCES ref.county (code),
    total_t        numeric NOT NULL,                -- 一般廢棄物產生量
    garbage_t      numeric NOT NULL,                -- 垃圾量（焚化、掩埋）
    recycled_t     numeric NOT NULL,                -- 資源回收量
    food_t         numeric NOT NULL,                -- 廚餘回收量
    per_capita_kg  numeric,                         -- 平均每人每日產生量（公斤）
    ingested_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (period_type, period, county_code)
);

GRANT SELECT ON data.vital_month, ref.childcare_center, data.power_sales_month, data.waste_county TO dashboard_reader;

-- +goose Down
DROP TABLE data.waste_county;
DROP TABLE data.power_sales_month;
DROP TABLE ref.childcare_center;
DROP TABLE data.vital_month;
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic IN ('family', 'sustain'));
DELETE FROM app.component WHERE topic IN ('family', 'sustain');
UPDATE app.api_survey SET topics = array_remove(array_remove(topics, 'family'), 'sustain')
 WHERE topics && ARRAY['family', 'sustain'];
DELETE FROM app.api_survey WHERE cardinality(topics) = 0;
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety'));
