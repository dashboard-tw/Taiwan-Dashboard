-- +goose Up
ALTER TABLE data.work_closure ADD COLUMN day date;
UPDATE data.work_closure SET day = (announced_at AT TIME ZONE 'Asia/Taipei')::date;
ALTER TABLE data.work_closure ALTER COLUMN day SET NOT NULL;
ALTER TABLE data.work_closure DROP CONSTRAINT work_closure_pkey;
ALTER TABLE data.work_closure ADD PRIMARY KEY (county_code, day, announced_at);

-- +goose Down
DELETE FROM data.work_closure WHERE day <> (announced_at AT TIME ZONE 'Asia/Taipei')::date;
ALTER TABLE data.work_closure DROP CONSTRAINT work_closure_pkey;
ALTER TABLE data.work_closure ADD PRIMARY KEY (county_code, announced_at);
ALTER TABLE data.work_closure DROP COLUMN day;
