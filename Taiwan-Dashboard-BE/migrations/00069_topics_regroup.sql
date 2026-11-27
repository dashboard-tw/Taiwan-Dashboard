-- +goose Up

-- 新頁面：用電統計、教育與托育；婦幼與家庭的元件分到人口和教育與托育（family 保留給舊資料）
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing', 'disease', 'aviation', 'economy', 'animal', 'forecast', 'health', 'ocean', 'flood',
                     'fuel', 'power-use', 'education'));

ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire', 'housing', 'disease', 'aviation', 'economy', 'animal',
                                                       'forecast', 'health', 'ocean', 'flood', 'fuel', 'power-use',
                                                       'education']);

DELETE FROM app.dashboard WHERE slug = 'family';

-- +goose Down
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food',
                                                       'fire', 'housing', 'disease', 'aviation', 'economy', 'animal',
                                                       'forecast', 'health', 'ocean', 'flood', 'fuel']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety', 'family', 'sustain', 'agri', 'food', 'fire',
                     'housing', 'disease', 'aviation', 'economy', 'animal', 'forecast', 'health', 'ocean', 'flood',
                     'fuel'));
