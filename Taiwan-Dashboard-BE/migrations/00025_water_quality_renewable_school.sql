-- +goose Up

CREATE TABLE ref.river_quality_site (
    site_id        text PRIMARY KEY,
    name           text NOT NULL,
    county_code    char(5) REFERENCES ref.county (code),
    township       text,
    basin          text,                            -- 流域，例如淡水河流域
    river          text,                            -- 河川，例如淡水河本流
    geom           geometry(Point, 4326),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data.river_quality (
    site_id      text NOT NULL REFERENCES ref.river_quality_site (site_id),
    sampled_at   timestamptz NOT NULL,
    rpi          numeric(4, 2) NOT NULL CHECK (rpi BETWEEN 1 AND 10),
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, sampled_at)
);
CREATE INDEX river_quality_sampled_idx ON data.river_quality (sampled_at);

CREATE VIEW data.river_quality_latest AS
SELECT DISTINCT ON (site_id) site_id, sampled_at, rpi
FROM data.river_quality
ORDER BY site_id, sampled_at DESC;

CREATE TABLE data.renewable_month (
    month        date NOT NULL,                     -- 該月 1 日
    source       text NOT NULL CHECK (source IN ('hydro', 'geothermal', 'solar', 'wind_onshore', 'wind_offshore',
                                                 'biomass_solid', 'biomass_gas', 'waste')),
    gen_mwh      numeric(12, 3) NOT NULL,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (month, source)
);

CREATE TABLE ref.school (
    school_id      text PRIMARY KEY,                -- 學制與教育部學校代碼，例如 elementary:011601
    level          text NOT NULL CHECK (level IN ('elementary', 'junior', 'senior', 'university')),
    code           text NOT NULL,
    name           text NOT NULL,
    is_public      boolean NOT NULL,
    county_code    char(5) REFERENCES ref.county (code),
    address        text,
    school_year    smallint NOT NULL,               -- 民國學年度
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX school_county_idx ON ref.school (county_code, level);

GRANT SELECT ON ref.river_quality_site, data.river_quality, data.river_quality_latest, data.renewable_month,
    ref.school TO dashboard_reader;

-- +goose Down
DROP TABLE ref.school;
DROP TABLE data.renewable_month;
DROP VIEW data.river_quality_latest;
DROP TABLE data.river_quality;
DROP TABLE ref.river_quality_site;
