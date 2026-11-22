-- +goose Up

ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic']);

CREATE TABLE data.disaster_alert (
    alert_id       text PRIMARY KEY,                -- CAP identifier
    kind           text NOT NULL CHECK (kind IN ('debris_flow', 'flood', 'flood_sensor', 'road_closure', 'reservoir_release')),
    msg_type       text NOT NULL CHECK (msg_type IN ('Alert', 'Update', 'Cancel')),
    sender         text NOT NULL,                   -- 發布機關，例如水利署、交通部公路局
    sent_at        timestamptz NOT NULL,
    effective_at   timestamptz,
    expires_at     timestamptz,
    headline       text NOT NULL,
    description    text,
    level          text,                            -- 警戒等級，例如紅色警戒、黃色警戒、解除警戒
    color          text,                            -- 警示顏色：紅色、橙色、黃色…
    areas          jsonb NOT NULL DEFAULT '[]',     -- [{desc, geocode}]：區域名稱與內政部的行政區代碼
    county_codes   char(5)[] NOT NULL DEFAULT '{}',
    geom           geometry(Point, 4326),           -- 有座標的訊息（淹水感測器、道路封閉）
    url            text,
    supersedes     text[] NOT NULL DEFAULT '{}',    -- 這則訊息取代的舊訊息
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX disaster_alert_seen_idx ON data.disaster_alert (last_seen_at);
CREATE INDEX disaster_alert_sent_idx ON data.disaster_alert (sent_at);
CREATE INDEX disaster_alert_supersedes_idx ON data.disaster_alert USING gin (supersedes);

CREATE VIEW data.disaster_alert_active AS
SELECT a.*
  FROM data.disaster_alert a
 WHERE a.last_seen_at >= now() - interval '30 minutes'
   AND a.msg_type <> 'Cancel'
   AND (a.expires_at IS NULL OR a.expires_at > now())
   AND NOT EXISTS (SELECT 1 FROM data.disaster_alert n WHERE a.alert_id = ANY (n.supersedes));

CREATE TABLE data.work_closure (
    county_code    char(5) NOT NULL REFERENCES ref.county (code),
    announced_at   timestamptz NOT NULL,            -- 頁面的更新時間
    status         text NOT NULL,                   -- 原文，例如「今天停止上班、停止上課。」
    level          smallint NOT NULL CHECK (level BETWEEN 0 AND 2), -- 0 照常、1 部分停止、2 停止上班上課
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (county_code, announced_at)
);
CREATE INDEX work_closure_seen_idx ON data.work_closure (last_seen_at);

CREATE TABLE ref.freeway_section (
    section_id    text PRIMARY KEY,                 -- 高速公路局路段代碼
    name          text NOT NULL,                    -- 例如「國道1號(基隆端至基隆)」
    road_name     text NOT NULL,
    direction     text,                             -- N、S、E、W
    start_name    text,
    end_name      text,
    length_km     numeric,
    speed_limit   smallint,
    county_code   char(5) REFERENCES ref.county (code), -- 路段中點所在的縣市
    geom          geometry(LineString, 4326),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.freeway_traffic_history (
    section_id     text NOT NULL REFERENCES ref.freeway_section (section_id),
    observed_at    timestamptz NOT NULL,
    speed_kmh      smallint,
    travel_time_s  integer,
    level          smallint,                        -- 1 順暢 … 5 壅塞，-1 道路封閉（高速公路局的分級）
    PRIMARY KEY (section_id, observed_at)
);
CREATE INDEX freeway_traffic_history_observed_at_idx ON data.freeway_traffic_history (observed_at);

CREATE TABLE data.freeway_traffic_current (LIKE data.freeway_traffic_history INCLUDING DEFAULTS);
ALTER TABLE data.freeway_traffic_current ADD PRIMARY KEY (section_id);

CREATE TABLE data.fuel_price (
    product       text NOT NULL,                    -- 例如 92無鉛汽油、超級柴油
    effective_at  timestamptz NOT NULL,
    price         numeric NOT NULL,
    unit          text,                             -- 例如「元/公升」
    ingested_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product, effective_at)
);

GRANT SELECT ON data.disaster_alert, data.disaster_alert_active, data.work_closure, ref.freeway_section,
                data.freeway_traffic_history, data.freeway_traffic_current, data.fuel_price
    TO dashboard_reader;

-- +goose Down
DROP TABLE data.fuel_price;
DROP TABLE data.freeway_traffic_current;
DROP TABLE data.freeway_traffic_history;
DROP TABLE ref.freeway_section;
DROP TABLE data.work_closure;
DROP VIEW data.disaster_alert_active;
DROP TABLE data.disaster_alert;
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic = 'traffic');
DELETE FROM app.component WHERE topic = 'traffic';
UPDATE app.api_survey SET topics = array_remove(topics, 'traffic') WHERE 'traffic' = ANY (topics);
DELETE FROM app.api_survey WHERE cardinality(topics) = 0;
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning'));
