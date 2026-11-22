-- +goose Up
SELECT set_config('app.rls_bypass', 'on', true);

UPDATE app.dashboard SET description = '全台的警特報、天氣、地震、空品、水情、用電、停水與鐵路通阻'
 WHERE slug = 'home';

DELETE FROM app.dashboard_component
 WHERE dashboard_id = (SELECT id FROM app.dashboard WHERE slug = 'home');

INSERT INTO app.dashboard_component (dashboard_id, component_id, position, size)
SELECT d.id, c.id, v.position, 'normal'
  FROM app.dashboard d
  JOIN (VALUES
    ('weather-warning-by-county', 0),
    ('county-forecast', 1),
    ('quake-recent', 2),
    ('aqi-by-county', 3),
    ('reservoir-by-county', 4),
    ('power-load-today', 5),
    ('water-outage-by-county', 6),
    ('rail-alert-list', 7)
  ) AS v (slug, position) ON true
  JOIN app.component c ON c.slug = v.slug
 WHERE d.slug = 'home';

-- +goose Down
SELECT set_config('app.rls_bypass', 'on', true);

UPDATE app.dashboard SET description = '各縣市的天氣、警特報、空品、水情與即時發電結構'
 WHERE slug = 'home';

DELETE FROM app.dashboard_component
 WHERE dashboard_id = (SELECT id FROM app.dashboard WHERE slug = 'home');

INSERT INTO app.dashboard_component (dashboard_id, component_id, position, size)
SELECT d.id, c.id, v.position, 'normal'
  FROM app.dashboard d
  JOIN (VALUES
    ('county-forecast', 0),
    ('weather-warning-by-county', 1),
    ('temp-by-county', 2),
    ('aqi-by-county', 3),
    ('reservoir-by-county', 4),
    ('power-gen-mix', 5)
  ) AS v (slug, position) ON true
  JOIN app.component c ON c.slug = v.slug
 WHERE d.slug = 'home';
