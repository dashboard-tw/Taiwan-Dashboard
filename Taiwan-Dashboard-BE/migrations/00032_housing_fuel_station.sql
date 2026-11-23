-- +goose Up
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire', 'housing']);

CREATE TABLE data.real_estate_deal (
    serial         text PRIMARY KEY,                -- 實價登錄的編號
    kind           text NOT NULL CHECK (kind IN ('sale', 'presale', 'rent')),
    season         text NOT NULL,                   -- 公布的季，例如 115S3
    deal_date      date NOT NULL,
    county_code    char(5) REFERENCES ref.county (code),
    district       text,
    target         text,                            -- 交易標的，例如房地(土地+建物)+車位
    building_type  text,                            -- 建物型態，例如住宅大樓
    main_use       text,                            -- 主要用途，例如住家用
    total_price    bigint,                          -- 元（租賃是每月租金）
    unit_price     numeric(12, 1),                  -- 元/平方公尺
    area_m2        numeric(12, 2),                  -- 建物移轉（租賃）總面積
    rooms          smallint,
    special        boolean NOT NULL DEFAULT false,  -- 備註有親友、員工等特殊關係，價格統計不列入
    ingested_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX real_estate_deal_kind_idx ON data.real_estate_deal (kind, deal_date);
CREATE INDEX real_estate_deal_county_idx ON data.real_estate_deal (county_code, kind, deal_date);
CREATE INDEX real_estate_deal_season_idx ON data.real_estate_deal (season);

CREATE TABLE ref.fuel_station (
    station_id     text PRIMARY KEY,
    name           text NOT NULL,
    kind           text,                            -- 直營站、加盟站
    county_code    char(5) REFERENCES ref.county (code),
    town           text,
    address        text,
    open_hours     text,
    operating      boolean NOT NULL,
    self_service   boolean NOT NULL,
    freeway        boolean NOT NULL,                -- 國道服務區
    has_98         boolean NOT NULL,
    has_diesel     boolean NOT NULL,
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fuel_station_county_idx ON ref.fuel_station (county_code);

GRANT SELECT ON data.real_estate_deal, ref.fuel_station TO dashboard_reader;

-- +goose Down
DROP TABLE ref.fuel_station;
DROP TABLE data.real_estate_deal;
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic = 'housing');
DELETE FROM app.component WHERE topic = 'housing';
UPDATE app.api_survey SET topics = array_remove(topics, 'housing') WHERE 'housing' = ANY (topics);
DELETE FROM app.api_survey WHERE cardinality(topics) = 0;
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire'));
