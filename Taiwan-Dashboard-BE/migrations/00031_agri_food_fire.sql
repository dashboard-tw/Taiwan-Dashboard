-- +goose Up
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire']);

CREATE TABLE data.farm_price_daily (
    trade_date   date NOT NULL,
    category     text NOT NULL CHECK (category IN ('vegetable', 'fruit', 'flower')),
    crop         text NOT NULL,                     -- 作物名稱去掉品種，例如甘藍
    avg_price    numeric(10, 2) NOT NULL,
    volume       numeric(14, 1) NOT NULL,
    markets      smallint NOT NULL,                 -- 有交易的市場數
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (trade_date, category, crop)
);
CREATE INDEX farm_price_daily_crop_idx ON data.farm_price_daily (category, crop, trade_date);

CREATE TABLE data.food_rejection (
    rejection_id  text PRIMARY KEY,                 -- 發布日期、產地、品名、原因的雜湊
    published_on  date NOT NULL,
    received_on   date,                             -- 報驗受理日期
    origin        text NOT NULL,                    -- 產地
    product       text NOT NULL,
    reason        text NOT NULL,
    hs_code       text,                             -- 貨品分類號列
    disposal      text,                             -- 處置情形，例如退運或銷毀
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX food_rejection_published_idx ON data.food_rejection (published_on);

CREATE TABLE data.fire_incident (
    case_id      text PRIMARY KEY,                  -- 消防署火災案件編號
    reported_at  timestamptz NOT NULL,
    arrived_at   timestamptz,
    county_code  char(5) REFERENCES ref.county (code),
    town         text,
    cause        text NOT NULL,                     -- 起火原因，統一寫法（例如電氣因素）
    deaths       smallint NOT NULL DEFAULT 0,
    injuries     smallint NOT NULL DEFAULT 0,
    ingested_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fire_incident_reported_idx ON data.fire_incident (reported_at);

GRANT SELECT ON data.farm_price_daily, data.food_rejection, data.fire_incident TO dashboard_reader;

-- +goose Down
DROP TABLE data.fire_incident;
DROP TABLE data.food_rejection;
DROP TABLE data.farm_price_daily;
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic IN ('agri', 'food', 'fire'));
DELETE FROM app.component WHERE topic IN ('agri', 'food', 'fire');
UPDATE app.api_survey SET topics = array_remove(array_remove(array_remove(topics, 'agri'), 'food'), 'fire')
 WHERE topics && ARRAY['agri', 'food', 'fire'];
DELETE FROM app.api_survey WHERE cardinality(topics) = 0;
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain'));
