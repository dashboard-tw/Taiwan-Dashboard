# 資料來源

每個子資料夾是一個資料來源的管線。排程寫在 `internal/cli/worker.go`。

| 資料集 id | 資料夾 | 來源 | 頻率 |
|---|---|---|---|
| `aqi_obs` | `aqi/` | 環境部 空氣品質指標（AQI）`aqx_p_432` | 每小時 |
| `uv_obs` | `aqi/` | 環境部 紫外線即時監測資料 `uv_s_01` | 每小時 |
| `aqi_forecast` | `aqi/` | 環境部 空氣品質預報資料 `aqf_p_01` | 每天三次 |
| `reservoir_ref` | `reservoir/` | 水利署 水庫每日營運狀況、水庫基本資料 | 每日 |
| `reservoir_obs` | `reservoir/` | 水利署 水庫水情資料 | 每小時 |
| `river_station` | `river/` | 水利署 河川水位測站站況（含警戒水位） | 每日 |
| `river_level` | `river/` | 水利署 即時水位資料 | 每 10 分鐘 |
| `water_outage` | `outage/` | 台灣自來水 停水資訊 | 每 30 分鐘 |
| `water_outage_taipei` | `outage/` | 臺北自來水事業處 停水公告 | 每 30 分鐘 |
| `power_outage` | `outage/` | 台電 電力維生管線災情資訊 `d006012`（災害期間） | 每 20 分鐘 |
| `earthquake` | `earthquake/` | 中央氣象署 顯著有感地震報告 `E-A0015-001` | 每 10 分鐘 |
| `earthquake_local` | `earthquake/` | 中央氣象署 小區域有感地震報告 `E-A0016-001` | 每 10 分鐘 |
| `typhoon_track` | `weather/` | 中央氣象署 熱帶氣旋路徑 `W-C0034-005` | 每 20 分鐘 |
| `typhoon_warning` | `weather/` | 中央氣象署 颱風警報 `W-C0034-001` | 每 15 分鐘 |
| `weather_warning` | `weather/` | 中央氣象署 各縣市天氣警特報 `W-C0033-001` | 每 10 分鐘 |
| `rain_obs` | `weather/` | 中央氣象署 自動雨量站雨量 `O-A0002-001` | 每 5 分鐘 |
| `weather_obs` | `weather/` | 中央氣象署 自動氣象站 `O-A0001-001` 與現在天氣觀測 `O-A0003-001`（合併） | 每 5 分鐘 |
| `county_forecast` | `weather/` | 中央氣象署 今明 36 小時天氣預報 `F-C0032-001` | 每 30 分鐘 |
| `wind_layer` | `weather/` | 中央氣象署 WRF 數值預報 `M-A0061`（GRIB2，只讀 10 公尺風，保留 7 天） | 每 30 分鐘 |
| `radar_image` | `weather/` | 中央氣象署 雷達合成回波圖 `O-A0058-003`（存成檔案，保留 7 天） | 每 5 分鐘 |
| `satellite_image` | `weather/` | 中央氣象署 衛星雲圖 `O-B0032-002`（存成檔案，保留 7 天） | 每 5 分鐘 |
| `radar_layer` | `weather/` | 中央氣象署 雷達合成回波格點 `O-A0059-001`（自己上色成地圖圖層，保留 7 天） | 每 5 分鐘 |
| `cloud_layer` | `weather/` | NASA GIBS 向日葵衛星紅外線 Band 13 圖磚（拼接後畫成雲，保留 7 天） | 每 10 分鐘 |
| `disaster_alert` | `alerts/` | 國家災害防救科技中心 民生示警（土石流、淹水、淹水感測、道路封閉、水庫放流） | 每 5 分鐘 |
| `work_closure` | `alerts/` | 人事行政總處 天然災害停止上班及上課情形 | 每 10 分鐘 |
| `freeway_section` | `traffic/` | 高速公路局 路段與線形 | 每天 |
| `freeway_traffic` | `traffic/` | 高速公路局 路段旅行速率 | 每 5 分鐘 |
| `fuel_price` | `fuel/` | 台灣中油 汽柴油歷史牌價 | 每 12 小時 |
| `traffic_accident` | `traffic/` | 警政署 A1 類道路交通事故（當年，不存當事人資料） | 每天（來源約每月） |
| `medical_facility` | `resources/` | 健保署 健保特約醫事機構（醫院、診所、藥局五個檔案） | 每天 |
| `ltc_facility` | `resources/` | 衛福部 長照 ABC 據點（不存負責人資料） | 每週 |
| `shelter` | `resources/` | 消防署 避難收容處所 | 每 30 分鐘 |
| `shelter_report` | `resources/` | 消防署 收容所開設通報（災害期間） | 每 10 分鐘 |
| `population_year` | `population/` | 戶政司 村里戶數、人口數按戶別及性別 `ODRP019`（加總成縣市） | 每週（來源每年） |
| `tourism_attraction` | `tourism/` | 觀光署 觀光資訊資料庫 景點 | 每天 |
| `tourism_hotel` | `tourism/` | 觀光署 觀光資訊資料庫 旅館民宿 | 每天 |
| `river_quality` | `river/` | 環境部 河川水質監測資料 `wqx_p_01`（只取 RPI） | 每天（來源每月） |
| `renewable_month` | `power/` | 能源署 再生能源發電量 | 每天（來源每月） |
| `school` | `school/` | 教育部統計處 國小、國中、高中、大專校院名錄 | 每週（來源每學年） |
| `rail_alert` | `rail/` | TDX 臺鐵與高鐵營運通阻 | 每 2 小時 |
| `tra_station` | `rail/` | TDX 臺鐵車站 | 每週 |
| `tra_live` | `rail/` | TDX 臺鐵列車即時位置與誤點 | 每小時 |
| `bike_station` | `bike/` | TDX 公共自行車站點（15 個縣市） | 每週 |
| `bike_availability` | `bike/` | TDX 公共自行車即時車位 | 每天 8:12、20:12 |
| `vital_month` | `population/` | 戶政司 各村(里)戶籍人口統計月報表 `ODRP010`（出生、死亡、結婚、離婚，加總成縣市） | 每天（來源每月） |
| `childcare_center` | `welfare/` | 衛福部 社會福利機構名冊中的托嬰中心 | 每週 |
| `power_sales` | `power/` | 台電 各縣市住宅、服務業及機關用電統計 `d007012` | 每天（來源每月） |
| `waste` | `waste/` | 環境部 執行機關一般廢棄物產生量 `stat_p_127` | 每天（來源每月） |
| `farm_price` | `agri/` | 農業部 農產品交易行情（果菜、花卉批發市場） | 每 8 小時 |
| `food_rejection` | `food/` | 食藥署 邊境查驗不合格食品（不存進口商） | 每天 |
| `fire_incident` | `fire/` | 消防署 火災事件資料（每年一個檔案） | 每週（來源每年） |
| `real_estate` | `housing/` | 內政部 不動產實價登錄，保留最近四季，不存門牌 | 每天（來源每季，每旬補登錄） |
| `fuel_station` | `fuel/` | 台灣中油 加油站服務資訊（不存電話） | 每天 |
| `dengue_daily` | `disease/` | 疾管署 登革熱近 12 個月每日確定病例（只存每天各鄉鎮的病例數） | 每 6 小時 |
| `nhi_visit` | `disease/` | 疾管署 健保門診、急診與住院就診人次（類流感、腸病毒、新冠） | 每天（來源每週） |
| `econ_indicator` | `economy/` | 主計總處 消費者物價、失業率，國發會 景氣指標及燈號 | 每天（來源每月） |
| `income_town` | `economy/` | 財政部 綜稅綜合所得總額各縣市鄉鎮村里統計（只存鄉鎮合計） | 每週（來源每年） |
| `shelter_animal` | `animal/` | 農業部 動物認領養（不轉載照片） | 每 6 小時 |
| `shelter_stats` | `animal/` | 農業部 全國公立動物收容所收容處理情形統計 | 每天（來源每月） |
| `airport` | `aviation/` | TDX 機場與航空公司 | 每週 |
| `flight` | `aviation/` | TDX 全國各機場航班動態 | 每 3 小時 |
| `debris_alert` | `debris/` | 農村水保署 土石流及大規模崩塌警戒 | 每 10 分鐘 |
| `flood_sensor` | `flood/` | 水利署 淹水感測器（民生公共物聯網 SensorThings） | 每 10 分鐘 |
| `ev_station` | `ev/` | TDX 電動車充電站 | 每週 |
| `ev_live` | `ev/` | TDX 電動車充電槍即時狀態 | 每天 12:22 |
| `poultry_price` | `agri/` | 農業部 家禽交易行情（雞蛋、白肉雞） | 每天 |
| `hog_trade` | `agri/` | 農業部 毛豬交易行情 | 每天 |
| `power_gen` | `power/` | 台電 各機組發電量即時資訊 | 每 10 分鐘 |
| `power_region` | `power/` | 台電 今日區域別用電情況 `d006019` | 每 10 分鐘 |
| `power_load` | `power/` | 台電 今日系統供需狀況 `d006020` | 每 10 分鐘 |
| `power_peak_daily` | `power/` | 台電 今日系統供需狀況 `d006020` 的昨日尖峰 | 每天 |
| `power_supply_daily` | `power/` | 台電 過去電力供需資訊 | 每月 |
| `power_supply_forecast` | `power/` | 台電 未來二個月電力供需預測 | 每 10 天 |

## TDX 額度

交通部 TDX 的免費會員每分鐘可以呼叫 5 次，每個月另有點數上限。點數用完後，當月會一直回應 `TDX token: HTTP 400`。費率見 [TDX 訂閱收費](https://tdx.transportdata.tw/pricing)。

TDX 管線的排程以每天約 120 次來分配：

| 資料集 | 每次呼叫 | 排程 | 每天 |
|---|---|---|---|
| `rail_alert` | 2 | 每 2 小時 | 24 |
| `tra_live` | 1 | 每小時 | 24 |
| `flight` | 2 | 每 3 小時 | 16 |
| `bike_availability` | 15 | 8:12、20:12 | 30 |
| `ev_live` | 21 | 12:22 | 21 |
| 四個主檔 | 1～21 | 每週 | 約 5.5 |

可以另外設定 `TDX_CLIENT_ID_2`、`TDX_CLIENT_SECRET_2`、`TDX_CLIENT_ID_3`、`TDX_CLIENT_SECRET_3`，TDX 管線會分給各帳號，各自計算額度。

| 帳號數 | 帳號 1 | 帳號 2 | 帳號 3 |
|---|---|---|---|
| 1 | 全部 | | |
| 2 | 臺鐵、高鐵、航班 | YouBike、充電站 | |
| 3 | 臺鐵、高鐵 | YouBike | 航班、充電站 |

調整排程前先算好次數。worker 每次啟動會把所有管線各跑一次，不要頻繁重新啟動。

## 測試資料

各管線的 `testdata/` 多數是從開放資料下載的真實資料節錄，另外加上幾筆異常值來測試解析。以下兩個檔案是依官方格式自己寫的：

- `outage/testdata/power.xml`：台電停電資訊，平時沒有資料可以下載
- `aqi/testdata/aqi.json`：環境部空品資料

真實資料依政府資料開放授權條款－第 1 版使用，來源機關包含經濟部水利署、台灣電力公司、環境部、台灣自來水公司、臺北自來水事業處與交通部中央氣象署。
