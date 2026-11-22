-- +goose Up
CREATE OR REPLACE VIEW data.disaster_alert_active AS
SELECT a.*
  FROM data.disaster_alert a
 WHERE a.last_seen_at >= coalesce(nullif(current_setting('dashboardtw.as_of', true), '')::timestamptz, now()) - interval '30 minutes'
   AND a.msg_type <> 'Cancel'
   AND (a.expires_at IS NULL OR a.expires_at > coalesce(nullif(current_setting('dashboardtw.as_of', true), '')::timestamptz, now()))
   AND NOT EXISTS (SELECT 1 FROM data.disaster_alert n WHERE a.alert_id = ANY (n.supersedes));

CREATE OR REPLACE VIEW data.rail_alert_active AS
SELECT *
FROM data.rail_alert
WHERE last_seen_at >= coalesce(nullif(current_setting('dashboardtw.as_of', true), '')::timestamptz, now()) - interval '130 minutes'
  AND (ends_at IS NULL OR ends_at > coalesce(nullif(current_setting('dashboardtw.as_of', true), '')::timestamptz, now()));

CREATE OR REPLACE VIEW data.debris_alert_active AS
    SELECT * FROM data.debris_alert
     WHERE last_seen_at >= coalesce(nullif(current_setting('dashboardtw.as_of', true), '')::timestamptz, now()) - interval '30 minutes';

-- +goose Down
CREATE OR REPLACE VIEW data.disaster_alert_active AS
SELECT a.*
  FROM data.disaster_alert a
 WHERE a.last_seen_at >= now() - interval '30 minutes'
   AND a.msg_type <> 'Cancel'
   AND (a.expires_at IS NULL OR a.expires_at > now())
   AND NOT EXISTS (SELECT 1 FROM data.disaster_alert n WHERE a.alert_id = ANY (n.supersedes));

CREATE OR REPLACE VIEW data.rail_alert_active AS
SELECT *
FROM data.rail_alert
WHERE last_seen_at >= now() - interval '30 minutes'
  AND (ends_at IS NULL OR ends_at > now());

CREATE OR REPLACE VIEW data.debris_alert_active AS
    SELECT * FROM data.debris_alert WHERE last_seen_at >= now() - interval '30 minutes';
