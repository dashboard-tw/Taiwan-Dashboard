-- +goose Up
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing', 'disease', 'aviation', 'economy', 'animal'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire', 'housing', 'disease', 'aviation', 'economy', 'animal']);

CREATE TABLE data.dengue_daily (
    onset_date   date NOT NULL,
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    town         text NOT NULL,
    imported     boolean NOT NULL,
    cases        integer NOT NULL,
    PRIMARY KEY (onset_date, county_code, town, imported)
);

CREATE TABLE data.nhi_visit_week (
    disease      text NOT NULL CHECK (disease IN ('ili', 'ev', 'covid')),
    year         smallint NOT NULL,
    week         smallint NOT NULL,
    week_start   date NOT NULL,                    -- 該週的星期一
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    visit_type   text NOT NULL CHECK (visit_type IN ('outpatient', 'emergency', 'inpatient')),
    visits       integer NOT NULL,                 -- 該疾病的就診人次（各年齡加總）
    total        integer NOT NULL,                 -- 全部就診人次
    PRIMARY KEY (disease, year, week, county_code, visit_type)
);
CREATE INDEX nhi_visit_week_start_idx ON data.nhi_visit_week (disease, week_start);

CREATE TABLE data.econ_month (
    series  text NOT NULL,
    month   date NOT NULL,                         -- 該月 1 日
    value   numeric NOT NULL,
    PRIMARY KEY (series, month)
);

CREATE TABLE data.income_town (
    year         smallint NOT NULL,                -- 所得年度（西元）
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    town         text NOT NULL,
    households   integer NOT NULL,                 -- 納稅單位
    income_k     bigint NOT NULL,                  -- 綜合所得總額
    mean_k       integer NOT NULL,
    median_k     integer NOT NULL,
    PRIMARY KEY (year, county_code, town)
);

CREATE TABLE data.shelter_animal (
    animal_id     bigint PRIMARY KEY,
    kind          text NOT NULL,                   -- 狗、貓、其他
    variety       text,
    sex           text,                            -- M、F、N（未輸入）
    age           text,                            -- CHILD、ADULT
    body_type     text,                            -- SMALL、MEDIUM、BIG
    colour        text,
    sterilized    boolean,
    shelter       text NOT NULL,
    county_code   char(5) REFERENCES ref.county (code),
    open_date     date,
    fetched_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX shelter_animal_county_idx ON data.shelter_animal (county_code);

CREATE TABLE data.shelter_month (
    county_code  char(5) NOT NULL REFERENCES ref.county (code),
    month        date NOT NULL,
    accepted     integer NOT NULL,
    adopted      integer NOT NULL,
    euthanized   integer NOT NULL,
    died         integer NOT NULL,
    PRIMARY KEY (county_code, month)
);

CREATE TABLE ref.airport (
    airport_id   text PRIMARY KEY,                 -- IATA，例如 TPE
    name         text NOT NULL,
    city         text,
    country      text,
    county_code  char(5) REFERENCES ref.county (code),
    geom         geometry(Point, 4326),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE ref.airline (
    airline_id   text PRIMARY KEY,
    name         text NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.flight (
    direction      text NOT NULL CHECK (direction IN ('dep', 'arr')),
    airport_id     text NOT NULL,
    airline_id     text NOT NULL,
    flight_no      text NOT NULL,
    scheduled_at   timestamptz NOT NULL,
    flight_date    date NOT NULL,
    other_airport  text,                           -- 出發是目的地，抵達是出發地
    estimated_at   timestamptz,
    actual_at      timestamptz,
    status         text NOT NULL CHECK (status IN ('on_time', 'delayed', 'cancelled', 'changed', 'done', 'extra', 'other')),
    remark         text,
    cargo          boolean NOT NULL,
    last_seen_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (direction, airport_id, airline_id, flight_no, scheduled_at)
);
CREATE INDEX flight_date_idx ON data.flight (flight_date, airport_id);

GRANT SELECT ON data.dengue_daily, data.nhi_visit_week, data.econ_month, data.income_town, data.shelter_animal,
    data.shelter_month, ref.airport, ref.airline, data.flight TO dashboard_reader;

-- +goose Down
DROP TABLE data.flight;
DROP TABLE ref.airline;
DROP TABLE ref.airport;
DROP TABLE data.shelter_month;
DROP TABLE data.shelter_animal;
DROP TABLE data.income_town;
DROP TABLE data.econ_month;
DROP TABLE data.nhi_visit_week;
DROP TABLE data.dengue_daily;
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic IN ('disease', 'aviation', 'economy', 'animal'));
DELETE FROM app.component WHERE topic IN ('disease', 'aviation', 'economy', 'animal');
UPDATE app.api_survey
   SET topics = array_remove(array_remove(array_remove(array_remove(topics, 'disease'), 'aviation'), 'economy'), 'animal')
 WHERE topics && ARRAY['disease', 'aviation', 'economy', 'animal'];
DELETE FROM app.api_survey WHERE cardinality(topics) = 0;
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire', 'housing']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing'));
