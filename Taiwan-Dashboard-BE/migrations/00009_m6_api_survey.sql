-- +goose Up
CREATE TABLE app.api_survey (
    user_id        bigint PRIMARY KEY REFERENCES app.users (id) ON DELETE CASCADE,
    identity       text NOT NULL
                   CHECK (identity IN ('individual', 'student', 'media', 'government', 'nonprofit', 'company', 'other')),
    purpose        text NOT NULL
                   CHECK (purpose IN ('research', 'journalism', 'app', 'analysis', 'teaching', 'personal', 'other')),
    app_name       text NOT NULL DEFAULT '' CHECK (char_length(app_name) <= 50),
    app_url        text NOT NULL DEFAULT '' CHECK (char_length(app_url) <= 200),
    topics         text[] NOT NULL
                   CHECK (cardinality(topics) > 0 AND topics <@ ARRAY['aqi', 'reservoir', 'power']),
    suggestion     text NOT NULL DEFAULT '' CHECK (char_length(suggestion) <= 500),
    terms_version  text NOT NULL,                   -- 同意的 API 使用規範版本
    agreed_at      timestamptz NOT NULL DEFAULT now(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE app.api_survey ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.api_survey FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.api_survey
    USING (app.rls_bypass() OR user_id = app.current_user_id());

-- +goose Down
DROP TABLE app.api_survey;
