# 縣市界線資料

`Taiwan-Dashboard-BE/migrations/00003_seed_counties.sql` 是用這個資料夾的 `county-seed.sql` 產生的。

## 資料來源

| 項目 | 內容 |
|---|---|
| 資料集 | [直轄市、縣市界線(TWD97經緯度)](https://data.gov.tw/dataset/7442) |
| 提供機關 | 內政部國土測繪中心 |
| 版本 | `COUNTY_MOI_1140318`（民國 114 年 3 月 18 日） |
| 授權 | 政府資料開放授權條款－第 1 版 |
| 座標系統 | TWD97 經緯度，與 WGS84 相差不到 1 公尺，直接當成 EPSG:4326 |

## 重新產生

內政部更新界線時才需要。需要 PostGIS 3.4 以上，在 PowerShell 執行：

1. 下載 SHP 版本並解壓縮到一個空資料夾
2. 設定 `DATABASE_URL`，指到要匯入的資料庫

3. 在解壓縮的資料夾裡，匯入到暫存 schema：

```bash
psql -c "DROP SCHEMA IF EXISTS import_tmp CASCADE; CREATE SCHEMA import_tmp;" -d $env:DATABASE_URL
```

```bash
shp2pgsql -s 4326 -W UTF-8 -g geom COUNTY_MOI_1140318.shp import_tmp.county | psql -q -v ON_ERROR_STOP=1 -d $env:DATABASE_URL
```

4. 產生 INSERT 內容，放進一個新的 migration。不要修改已經套用的 `00003`：

```bash
psql -tA -f scripts\geo\county-seed.sql -d $env:DATABASE_URL
```

5. 清除暫存 schema：

```bash
psql -c "DROP SCHEMA import_tmp CASCADE;" -d $env:DATABASE_URL
```
