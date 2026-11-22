-- +goose Up
ALTER TABLE app.api_usage_daily ADD COLUMN rejected integer NOT NULL DEFAULT 0;
COMMENT ON COLUMN app.api_usage_daily.rejected IS '超過每分鐘或每天額度而被拒絕的請求數（不算進 requests）';

CREATE TABLE app.api_usage_dataset_daily (
    day       date NOT NULL,
    dataset   text NOT NULL,                        -- 資料集 id；清單是 _list
    requests  integer NOT NULL DEFAULT 0,
    PRIMARY KEY (day, dataset)
);

-- +goose Down
DROP TABLE app.api_usage_dataset_daily;
ALTER TABLE app.api_usage_daily DROP COLUMN rejected;
