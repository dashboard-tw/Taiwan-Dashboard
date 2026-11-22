-- +goose Up
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism', 'rail', 'bike', 'safety'));
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism',
                                                       'rail', 'bike', 'safety']);

-- +goose Down
UPDATE app.component SET topic = 'traffic' WHERE topic IN ('rail', 'bike', 'safety');
UPDATE app.api_survey SET topics = array_remove(array_remove(array_remove(topics, 'rail'), 'bike'), 'safety')
 WHERE topics && ARRAY['rail', 'bike', 'safety'];
DELETE FROM app.api_survey WHERE cardinality(topics) = 0;
ALTER TABLE app.api_survey DROP CONSTRAINT api_survey_topics_check;
ALTER TABLE app.api_survey ADD CONSTRAINT api_survey_topics_check
    CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather',
                                                       'warning', 'traffic', 'resources', 'population', 'tourism']);
ALTER TABLE app.component DROP CONSTRAINT component_topic_check;
ALTER TABLE app.component ADD CONSTRAINT component_topic_check
    CHECK (topic IN ('aqi', 'reservoir', 'power', 'outage', 'earthquake', 'weather', 'warning', 'traffic', 'resources',
                     'population', 'tourism'));
