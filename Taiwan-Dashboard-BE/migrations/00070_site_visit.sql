-- +goose Up

-- 網站瀏覽人次：每個瀏覽器每次開啟網站算一次，只存每天的總數
CREATE TABLE app.site_visit_daily (
    day     date PRIMARY KEY,
    visits  bigint NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE app.site_visit_daily;
