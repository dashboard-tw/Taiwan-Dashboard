-- +goose Up
SELECT set_config('app.rls_bypass', 'on', true);

UPDATE app.dashboard SET description = '各縣市最大震度、近期的有感地震、各縣市的有感次數與震度分布'
 WHERE slug = 'earthquake';

DELETE FROM app.dashboard_component
 WHERE dashboard_id IN (SELECT id FROM app.dashboard WHERE slug IN ('home', 'earthquake'));

INSERT INTO app.dashboard_component (dashboard_id, component_id, position, size)
SELECT d.id, c.id, v.position, 'normal'
  FROM (VALUES
    ('home', 'weather-warning-by-county', 0),
    ('home', 'county-forecast', 1),
    ('home', 'quake-max-intensity-by-county', 2),
    ('home', 'aqi-by-county', 3),
    ('home', 'reservoir-by-county', 4),
    ('home', 'power-load-today', 5),
    ('home', 'water-outage-by-county', 6),
    ('home', 'rail-alert-list', 7),
    ('earthquake', 'quake-max-intensity-by-county', 0),
    ('earthquake', 'quake-recent', 1),
    ('earthquake', 'quake-felt-by-county', 2),
    ('earthquake', 'quake-intensity-dist', 3)
  ) AS v (dashboard, slug, position)
  JOIN app.dashboard d ON d.slug = v.dashboard
  JOIN app.component c ON c.slug = v.slug;

-- +goose Down
SELECT set_config('app.rls_bypass', 'on', true);

UPDATE app.dashboard SET description = '近期的有感地震、各縣市的有感次數與震度分布'
 WHERE slug = 'earthquake';

DELETE FROM app.dashboard_component
 WHERE dashboard_id IN (SELECT id FROM app.dashboard WHERE slug IN ('home', 'earthquake'));

INSERT INTO app.dashboard_component (dashboard_id, component_id, position, size)
SELECT d.id, c.id, v.position, 'normal'
  FROM (VALUES
    ('home', 'weather-warning-by-county', 0),
    ('home', 'county-forecast', 1),
    ('home', 'quake-recent', 2),
    ('home', 'aqi-by-county', 3),
    ('home', 'reservoir-by-county', 4),
    ('home', 'power-load-today', 5),
    ('home', 'water-outage-by-county', 6),
    ('home', 'rail-alert-list', 7),
    ('earthquake', 'quake-recent', 0),
    ('earthquake', 'quake-felt-by-county', 1),
    ('earthquake', 'quake-intensity-dist', 2)
  ) AS v (dashboard, slug, position)
  JOIN app.dashboard d ON d.slug = v.dashboard
  JOIN app.component c ON c.slug = v.slug;
