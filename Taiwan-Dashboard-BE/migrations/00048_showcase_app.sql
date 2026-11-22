-- +goose Up
CREATE TABLE app.showcase_app (
    id           bigserial PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES app.users (id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40),
    description  text NOT NULL CHECK (char_length(description) BETWEEN 1 AND 200),
    web_url      text NOT NULL DEFAULT '' CHECK (char_length(web_url) <= 200),
    ios_url      text NOT NULL DEFAULT '' CHECK (char_length(ios_url) <= 200),
    android_url  text NOT NULL DEFAULT '' CHECK (char_length(android_url) <= 200),
    image        text NOT NULL DEFAULT '' CHECK (image ~ '^([0-9a-f]{32}\.(png|jpg))?$'),
    topics       text[] NOT NULL CHECK (cardinality(topics) BETWEEN 1 AND 10),
    show_author  boolean NOT NULL DEFAULT true,
    status       text NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'approved', 'rejected', 'removed')),
    review_note  text NOT NULL DEFAULT '' CHECK (char_length(review_note) <= 200),
    published    jsonb,
    published_at timestamptz,
    reviewed_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (web_url <> '' OR ios_url <> '' OR android_url <> '')
);

CREATE INDEX showcase_app_user_idx ON app.showcase_app (user_id);
CREATE INDEX showcase_app_published_idx ON app.showcase_app (published_at DESC) WHERE published IS NOT NULL;

ALTER TABLE app.showcase_app ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.showcase_app FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.showcase_app
    USING (app.rls_bypass() OR user_id = app.current_user_id());

-- +goose Down
DROP TABLE app.showcase_app;
