-- +goose Up
ALTER TABLE app.showcase_app
    ADD COLUMN category text NOT NULL DEFAULT 'web' CHECK (category IN ('app', 'web', 'innovation'));

UPDATE app.showcase_app SET category = 'app' WHERE web_url = '' AND (ios_url <> '' OR android_url <> '');
UPDATE app.showcase_app SET published = published || jsonb_build_object('category', category)
 WHERE published IS NOT NULL;

ALTER TABLE app.showcase_app ALTER COLUMN category DROP DEFAULT;

-- +goose Down
ALTER TABLE app.showcase_app DROP COLUMN category;
