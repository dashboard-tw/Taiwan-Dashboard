-- +goose Up
ALTER TABLE app.showcase_app
    ADD COLUMN screenshot text NOT NULL DEFAULT '' CHECK (screenshot ~ '^([0-9a-f]{32}\.(png|jpg))?$');

-- +goose Down
ALTER TABLE app.showcase_app DROP COLUMN screenshot;
