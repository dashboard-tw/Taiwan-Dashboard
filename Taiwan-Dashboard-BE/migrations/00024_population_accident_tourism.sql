-- +goose Up

ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources', 'population', 'tourism'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources', 'population', 'tourism']);

CREATE TABLE data.population_year (
    year          smallint NOT NULL,                -- 西元年
    county_code   char(5) NOT NULL REFERENCES ref.county (code),
    households    integer NOT NULL,
    population    integer NOT NULL,
    male          integer NOT NULL,
    female        integer NOT NULL,
    ingested_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (year, county_code)
);

CREATE TABLE data.traffic_accident (
    accident_id    text PRIMARY KEY,                -- 發生時間與地點的雜湊
    occurred_at    timestamptz NOT NULL,
    county_code    char(5) REFERENCES ref.county (code),
    police         text,                            -- 處理的警察局
    place          text NOT NULL,
    weather        text,
    light          text,
    road_class     text,                            -- 道路類別，例如市區道路、省道
    speed_limit    smallint,
    accident_type  text,                            -- 事故類型，例如車與車、人與車、車輛本身
    cause          text,                            -- 主要肇因
    vehicle        text,                            -- 第一當事者的車種
    deaths         smallint NOT NULL,
    injuries       smallint NOT NULL,
    geom           geometry(Point, 4326),
    ingested_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX traffic_accident_occurred_idx ON data.traffic_accident (occurred_at);
CREATE INDEX traffic_accident_county_idx ON data.traffic_accident (county_code, occurred_at);

CREATE TABLE ref.tourism_attraction (
    attraction_id  text PRIMARY KEY,
    name           text NOT NULL,
    classes        smallint[] NOT NULL DEFAULT '{}', -- 觀光署的景點分類代碼
    county_code    char(5) REFERENCES ref.county (code),
    town           text,
    address        text,
    description    text,                            -- 前 200 字
    free           boolean,                         -- 免費參觀
    status         smallint,                        -- 觀光署的服務狀態代碼，1 是正常開放
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tourism_attraction_county_idx ON ref.tourism_attraction (county_code);

CREATE TABLE ref.tourism_hotel (
    hotel_id       text PRIMARY KEY,
    name           text NOT NULL,
    class          smallint CHECK (class BETWEEN 1 AND 4),
    stars          smallint,
    rooms          integer,
    capacity       integer,
    lowest_price   integer,
    highest_price  integer,
    county_code    char(5) REFERENCES ref.county (code),
    town           text,
    address        text,
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tourism_hotel_county_idx ON ref.tourism_hotel (county_code);

GRANT SELECT ON data.population_year, data.traffic_accident, ref.tourism_attraction, ref.tourism_hotel
    TO dashboard_reader;

-- +goose Down
DROP TABLE ref.tourism_hotel;
DROP TABLE ref.tourism_attraction;
DROP TABLE data.traffic_accident;
DROP TABLE data.population_year;
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic IN ('population', 'tourism'));
DELETE FROM app.component WHERE topic IN ('population', 'tourism');
UPDATE app.api_survey SET topics = array_remove(array_remove(topics, 'population'), 'tourism')
 WHERE topics && ARRAY['population', 'tourism'];
DELETE FROM app.api_survey WHERE cardinality(topics) = 0;
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources']);
