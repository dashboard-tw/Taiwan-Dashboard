-- +goose Up
CREATE VIEW data.typhoon_event AS
WITH w AS (
    SELECT t.typhoon_id, w.*
    FROM data.typhoon t
    JOIN data.typhoon_warning w ON w.ty_no = t.ty_no
     AND w.effective_at >= make_timestamptz(t.year, 1, 1, 0, 0, 0, 'Asia/Taipei')
     AND w.effective_at < make_timestamptz(t.year + 1, 1, 1, 0, 0, 0, 'Asia/Taipei')
), agg AS (
    SELECT typhoon_id, min(effective_at) AS starts_at, count(*) AS warnings,
           (array_agg(name_zh ORDER BY effective_at DESC) FILTER (WHERE name_zh IS NOT NULL))[1] AS warning_name
    FROM w
    GROUP BY typhoon_id
), last AS (
    SELECT DISTINCT ON (typhoon_id) typhoon_id, alert, effective_at, expires_at
    FROM w
    ORDER BY typhoon_id, effective_at DESC
)
SELECT t.typhoon_id, t.ty_no, t.year, coalesce(t.name_zh, a.warning_name) AS name_zh, t.name_en,
       a.starts_at,
       CASE WHEN l.alert LIKE '%解除%' THEN l.effective_at
            WHEN coalesce(l.expires_at, l.effective_at + interval '6 hours') < now() - interval '1 hour'
                THEN coalesce(l.expires_at, l.effective_at + interval '6 hours')
       END AS ends_at,
       l.alert AS last_alert, a.warnings::integer AS warnings
FROM data.typhoon t
JOIN agg a USING (typhoon_id)
JOIN last l USING (typhoon_id);

GRANT SELECT ON data.typhoon_event TO dashboard_reader;

-- +goose Down
DROP VIEW data.typhoon_event;
