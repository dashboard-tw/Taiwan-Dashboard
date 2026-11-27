-- +goose Up

-- 新頁面：土石流與淹水、加油與充電；交通安全併入道路交通（safety 保留給舊資料）
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing', 'disease', 'aviation', 'economy', 'animal', 'forecast', 'health', 'ocean', 'flood',
                     'fuel'));

-- 使用調查的主題跟著頁面走（之前少了天氣預報、健康氣象、海洋與天文）
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire', 'housing', 'disease', 'aviation', 'economy', 'animal',
                                                       'forecast', 'health', 'ocean', 'flood', 'fuel']);

-- 交通安全頁的元件併到道路交通
DELETE FROM app.dashboard WHERE slug = 'safety';

-- +goose Down
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire', 'housing', 'disease', 'aviation', 'economy', 'animal']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing', 'disease', 'aviation', 'economy', 'animal', 'forecast', 'health', 'ocean'));
