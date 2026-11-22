# 範例資料

`sample.sql.gz` 讓本機啟動後就有畫面，用 docker 啟動時會自動載入。

內容是測站、水庫、學校等參考資料，加上各資料集最新的一筆數值。全部來自政府開放資料，沒有使用者資料。

範例不含歷史資料，啟動 worker 後才會開始累積。

手動載入前，先用 `dashboardtw migrate up` 建好資料表：

```bash
gunzip -c sample.sql.gz | psql "$DATABASE_URL"
```
