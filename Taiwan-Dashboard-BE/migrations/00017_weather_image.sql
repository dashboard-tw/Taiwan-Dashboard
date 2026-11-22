-- +goose Up
CREATE TABLE data.weather_image (
    product      text NOT NULL CHECK (product IN ('radar', 'satellite')),
    observed_at  timestamptz NOT NULL,
    file         text NOT NULL,                     -- IMAGE_DIR 底下的相對路徑，例如 radar/20261022T0930Z.jpg
    width        integer NOT NULL,
    height       integer NOT NULL,
    bytes        integer NOT NULL,
    sha256       text NOT NULL,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product, observed_at)
);

GRANT SELECT ON data.weather_image TO dashboard_reader;

-- +goose Down
DROP TABLE data.weather_image;
