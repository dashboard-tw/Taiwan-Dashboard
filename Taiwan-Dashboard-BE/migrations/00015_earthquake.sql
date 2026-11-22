-- +goose Up

ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake']);

CREATE TABLE data.earthquake (
    report_id           text PRIMARY KEY,           -- 報告網址最後一段，例如 2026069（顯著）、2026102114512039（小區域）
    kind                text NOT NULL CHECK (kind IN ('significant', 'local')),
    earthquake_no       integer,                    -- 地震編號；小區域報告都是 115000 這類沒有編號的值
    origin_time         timestamptz NOT NULL,       -- 發震時間
    issued_at           timestamptz,                -- 報告發布時間
    magnitude           numeric,                    -- 芮氏規模
    depth_km            numeric,                    -- 震源深度（公里）
    location            text,                       -- 例如「花蓮縣政府南南西方 50.8 公里 (位於花蓮縣豐濱鄉)」
    geom                geometry(Point, 4326),      -- 震央
    max_intensity       text,                       -- 各縣市中最大的震度，例如 3級、5弱
    max_intensity_rank  smallint,                   -- 震度的順序：1級=1 … 4級=4、5弱=5、5強=6、6弱=7、6強=8、7級=9
    report_color        text,                       -- 綠色、黃色、橘色、紅色
    content             text,                       -- 報告內容（一句話摘要）
    report_url          text,
    image_url           text,
    ingested_at         timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX earthquake_origin_time_idx ON data.earthquake (origin_time);

CREATE TABLE data.earthquake_intensity (
    report_id       text NOT NULL REFERENCES data.earthquake (report_id) ON DELETE CASCADE,
    county_code     char(5) NOT NULL REFERENCES ref.county (code),
    intensity       text NOT NULL,
    intensity_rank  smallint NOT NULL,
    PRIMARY KEY (report_id, county_code)
);

GRANT SELECT ON data.earthquake, data.earthquake_intensity TO dashboard_reader;

-- +goose Down
DROP TABLE data.earthquake_intensity;
DROP TABLE data.earthquake;
SELECT set_config('app.rls_bypass', 'on', true);
DELETE FROM app.dashboard_component
 WHERE component_id IN (SELECT id FROM app.component WHERE topic = 'earthquake');
DELETE FROM app.dashboard WHERE slug = 'earthquake';
DELETE FROM app.component WHERE topic = 'earthquake';
UPDATE app.api_survey SET topics = array_remove(topics, 'earthquake') WHERE 'earthquake' = ANY (topics);
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage'));
