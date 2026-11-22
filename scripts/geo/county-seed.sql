-- 從內政部「直轄市、縣市界線(TWD97經緯度)」產生 backend/migrations/00003_seed_counties.sql 的 INSERT 內容。
-- 使用方式見 scripts/geo/README.md。前提：SHP 已用 shp2pgsql 匯入 import_tmp.county。
--
-- 簡化：ST_CoverageSimplify 把 22 縣市當成一整片一起簡化，相鄰邊界不會出現縫隙或重疊。
-- 容差 0.0003 度（約 30 公尺），足夠縣市層級的地圖與比對；首頁 API 會再進一步簡化。

WITH meta (code, short_name, aliases, sort_order) AS (
    VALUES
        ('63000', '臺北', ARRAY['台北市', '台北', '臺北'], 1),
        ('65000', '新北', ARRAY['新北'], 2),
        ('10017', '基隆', ARRAY['基隆巿', '基隆'], 3),
        ('68000', '桃園', ARRAY['桃園'], 4),
        ('10018', '新竹市', ARRAY['新竹巿'], 5),
        ('10004', '新竹縣', ARRAY[]::text[], 6),
        ('10005', '苗栗', ARRAY['苗栗'], 7),
        ('66000', '臺中', ARRAY['台中市', '台中', '臺中'], 8),
        ('10007', '彰化', ARRAY['彰化'], 9),
        ('10008', '南投', ARRAY['南投'], 10),
        ('10009', '雲林', ARRAY['雲林'], 11),
        ('10020', '嘉義市', ARRAY['嘉義巿'], 12),
        ('10010', '嘉義縣', ARRAY[]::text[], 13),
        ('67000', '臺南', ARRAY['台南市', '台南', '臺南'], 14),
        ('64000', '高雄', ARRAY['高雄巿', '高雄'], 15),
        ('10013', '屏東', ARRAY['屏東'], 16),
        ('10002', '宜蘭', ARRAY['宜蘭'], 17),
        ('10015', '花蓮', ARRAY['花蓮'], 18),
        ('10014', '臺東', ARRAY['台東縣', '台東', '臺東'], 19),
        ('10016', '澎湖', ARRAY['澎湖'], 20),
        ('09020', '金門', ARRAY['金門'], 21),
        ('09007', '連江', ARRAY['連江', '馬祖'], 22)
),
simplified AS (
    SELECT countycode, countyname, countyeng,
           ST_Multi(ST_CollectionExtract(ST_CoverageSimplify(geom, 0.0003) OVER (), 3)) AS g
    FROM import_tmp.county
),
centered AS (
    SELECT s.*,
           -- 標籤放在面積最大的那一塊（例如高雄市不要落在東沙島）
           (SELECT ST_PointOnSurface(d.geom)
              FROM ST_Dump(s.g) d
             ORDER BY ST_Area(d.geom) DESC
             LIMIT 1) AS c
    FROM simplified s
)
SELECT format(
    E'INSERT INTO ref.county (code, name, short_name, name_en, aliases, boundary, center, sort_order) VALUES (%L, %L, %L, %L, %L, ST_GeomFromGeoJSON(%L), ST_GeomFromGeoJSON(%L), %s);',
    c.countycode, c.countyname, m.short_name, c.countyeng, m.aliases,
    ST_AsGeoJSON(c.g, 6), ST_AsGeoJSON(c.c, 6), m.sort_order)
FROM centered c
JOIN meta m ON m.code = c.countycode
ORDER BY m.sort_order;
