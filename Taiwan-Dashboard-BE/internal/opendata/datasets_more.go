package opendata

func listLatest(cols, from, where, key string) string {
	return `
		SELECT * FROM (
		  SELECT ` + cols + `, ` + key + ` AS cursor_k
		    FROM ` + from + `
		   WHERE ` + where + `) q
		 WHERE ($3::text IS NULL OR cursor_k > $3)
		 ORDER BY cursor_k LIMIT $4`
}

func history(cols, from, where, timeExpr, key string) string {
	return `
		SELECT * FROM (
		  SELECT ` + cols + `,
		         ` + timeExpr + ` AS cursor_t, ` + key + ` AS cursor_k
		    FROM ` + from + `
		   WHERE ` + where + `
		     AND ` + timeExpr + ` >= $3 AND ` + timeExpr + ` < $4) q
		 WHERE ` + pageCond + `
		 ORDER BY cursor_t, cursor_k LIMIT $7`
}

func months(timeExpr, from string) string {
	return `SELECT to_char(` + timeExpr + ` AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM ` + from + ` GROUP BY 1 ORDER BY 1`
}

func dayStart(col string) string { return `(` + col + `::timestamp AT TIME ZONE 'Asia/Taipei')` }

func yearEnd(col string) string {
	return `make_timestamptz(` + col + `::int, 12, 31, 0, 0, 0, 'Asia/Taipei')`
}

const (
	noFilter   = `$1::text IS NULL AND $2::text IS NULL`
	countyOnly = countyCond + ` AND $2::text IS NULL`
	lonLat     = `ST_X(s.geom), ST_Y(s.geom)`
)

var (
	lonField = Field{"lon", "number", "度", "經度（WGS 84）"}
	latField = Field{"lat", "number", "度", "緯度（WGS 84）"}
)

func imageDataset(id, product, description string, bounds bool) Dataset {
	fields := []Field{
		{"observed_at", "timestamp", "", "影像的觀測時間（預報圖層是預報的時間）"},
		{"url", "string", "", "影像網址（本站路徑，前面加上 https://dashboard.tw，不需要 Access Token）"},
		{"width", "integer", "像素", "寬度"},
		{"height", "integer", "像素", "高度"},
	}
	cols := `w.observed_at, '/api/v1/images/' || w.file, w.width, w.height`
	if bounds {
		fields = append(fields,
			Field{"west", "number", "度", "影像涵蓋範圍的西界經度"},
			Field{"south", "number", "度", "南界緯度"},
			Field{"east", "number", "度", "東界經度"},
			Field{"north", "number", "度", "北界緯度"},
		)
		cols += `, w.west::float8, w.south::float8, w.east::float8, w.north::float8`
	}
	where := `w.product = '` + product + `' AND ` + noFilter
	return Dataset{
		ID:          id,
		Description: description + "最新一張由 latest 取得，最近 7 天的每一張由 history 取得。影像只保留 7 天，所以沒有大量下載。",
		Fields:      fields,
		LatestSQL:   `SELECT ` + cols + ` FROM data.weather_image w WHERE ` + where + ` ORDER BY w.observed_at DESC LIMIT 1`,
		HistorySQL:  history(cols, `data.weather_image w`, where, `w.observed_at`, `''`),
	}
}

var moreDatasets = []Dataset{
	{
		ID: "disaster_alert",
		Description: "國家災害防救科技中心彙整的民生示警（土石流、淹水、淹水感測、道路封閉、水庫放流）。每則訊息一列。" +
			"更新與解除是新的訊息，supersedes 列出它取代的舊訊息。latest 是目前生效中的訊息。歷史資料依發布時間查詢。",
		HasCounty: true,
		Fields: []Field{
			{"alert_id", "string", "", "訊息代碼（CAP identifier）"},
			{"kind", "string", "", "debris_flow 土石流、flood 淹水、flood_sensor 淹水感測、road_closure 道路封閉、reservoir_release 水庫放流"},
			{"msg_type", "string", "", "Alert 發布、Update 更新、Cancel 解除"},
			{"sender", "string", "", "發布機關"},
			{"sent_at", "timestamp", "", "發布時間"},
			{"effective_at", "timestamp", "", "生效時間"},
			{"expires_at", "timestamp", "", "失效時間"},
			{"headline", "string", "", "標題"},
			{"description", "string", "", "內容"},
			{"level", "string", "", "警戒等級，例如紅色警戒"},
			{"color", "string", "", "警示顏色"},
			{"county_codes", "string", "", "影響縣市代碼（以逗號分隔）"},
			{"lon", "number", "度", "經度（有座標的訊息才有，例如淹水感測器、道路封閉）"},
			{"lat", "number", "度", "緯度"},
			{"url", "string", "", "來源網址"},
			{"supersedes", "string", "", "這則訊息取代的舊訊息代碼（以逗號分隔）"},
			{"first_seen_at", "timestamp", "", "本站第一次抓到的時間"},
			{"last_seen_at", "timestamp", "", "最後一次出現在來源的時間"},
		},
		LatestSQL: `
			SELECT a.alert_id, a.kind, a.msg_type, a.sender, a.sent_at, a.effective_at, a.expires_at, a.headline, a.description,
			       a.level, a.color, array_to_string(a.county_codes, ','), ST_X(a.geom), ST_Y(a.geom), a.url,
			       array_to_string(a.supersedes, ','), a.first_seen_at, a.last_seen_at
			  FROM data.disaster_alert_active a
			 WHERE ($1::text IS NULL OR a.county_codes::text[] @> ARRAY[$1::text]) AND $2::text IS NULL
			 ORDER BY a.sent_at DESC`,
		HistorySQL: history(`a.alert_id, a.kind, a.msg_type, a.sender, a.sent_at, a.effective_at, a.expires_at, a.headline, a.description,
			       a.level, a.color, array_to_string(a.county_codes, ','), ST_X(a.geom), ST_Y(a.geom), a.url,
			       array_to_string(a.supersedes, ','), a.first_seen_at, a.last_seen_at`,
			`data.disaster_alert a`, `($1::text IS NULL OR a.county_codes::text[] @> ARRAY[$1::text]) AND $2::text IS NULL`,
			`a.sent_at`, `a.alert_id`),
		Months: months(`sent_at`, `data.disaster_alert`),
	},
	{
		ID: "debris_alert",
		Description: "農業部農村發展及水土保持署的土石流（D）與大規模崩塌（L）警戒：一條潛勢溪流或一個崩塌區一列。" +
			"latest 是最近 30 分鐘還在來源上的警戒。歷史資料依本站第一次抓到的時間查詢。",
		HasCounty: true,
		Fields: []Field{
			{"alert_type", "string", "", "D 土石流、L 大規模崩塌"},
			{"alert_id", "string", "", "土石流潛勢溪流編號或崩塌區編號"},
			{"name", "string", "", "崩塌區名稱"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"village", "string", "", "村里"},
			{"level", "string", "", "yellow 黃色警戒、red 紅色警戒"},
			{"updated_at", "timestamp", "", "來源的更新時間"},
			{"first_seen_at", "timestamp", "", "本站第一次抓到的時間"},
			{"last_seen_at", "timestamp", "", "最後一次出現在來源的時間"},
		},
		LatestSQL: `
			SELECT s.alert_type, s.alert_id, s.name, s.county_code, s.town, s.village, s.level, s.updated_at, s.first_seen_at, s.last_seen_at
			  FROM data.debris_alert_active s
			 WHERE ` + countyOnly + `
			 ORDER BY s.county_code, s.alert_type, s.alert_id`,
		HistorySQL: history(`s.alert_type, s.alert_id, s.name, s.county_code, s.town, s.village, s.level, s.updated_at, s.first_seen_at, s.last_seen_at`,
			`data.debris_alert s`, countyOnly, `s.first_seen_at`, `s.alert_type || s.alert_id`),
		Months: months(`first_seen_at`, `data.debris_alert`),
	},
	{
		ID:          "flood_sensor",
		Description: "民生公共物聯網的淹水感測器。latest 是每個感測器最新的淹水深度。歷史資料只保留積淹水（10 公分以上）的觀測，保留一年。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"station_code", "string", "", "感測器代碼"},
			{"name", "string", "", "名稱"},
			{"authority", "string", "", "管理單位"},
			countyField, lonField, latField,
		},
		SitesSQL: `
			SELECT s.station_code, s.name, s.authority, s.county_code, ` + lonLat + `
			  FROM ref.flood_sensor s WHERE ` + countyCond + ` ORDER BY s.station_code`,
		Fields: []Field{
			{"station_code", "string", "", "感測器代碼"},
			{"observed_at", "timestamp", "", "觀測時間"},
			{"depth_cm", "number", "公分", "淹水深度"},
		},
		LatestSQL: `
			SELECT o.station_code, o.observed_at, o.depth_cm::float8
			  FROM data.flood_current o JOIN ref.flood_sensor s USING (station_code)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.station_code = $2)
			 ORDER BY o.station_code`,
		HistorySQL: history(`o.station_code, o.observed_at, o.depth_cm::float8`,
			`data.flood_obs o JOIN ref.flood_sensor s USING (station_code)`,
			countyCond+` AND ($2::text IS NULL OR o.station_code = $2)`, `o.observed_at`, `o.station_code`),
		Months: months(`observed_at`, `data.flood_obs`),
	},
	{
		ID: "work_closure",
		Description: "行政院人事行政總處公布的各縣市天然災害停止上班及上課情形。內容改變時才新增一列。day 是公告適用的日期（晚上會同時公布明天）。" +
			"latest 是今天與之後每個縣市最新的公告。歷史資料依公告時間查詢。",
		HasCounty: true,
		Fields: []Field{
			countyField,
			{"day", "date", "", "公告適用的日期"},
			{"announced_at", "timestamp", "", "公告時間"},
			{"status", "string", "", "公告原文，例如「今天停止上班、停止上課。」"},
			{"level", "integer", "", "0 照常、1 部分停止、2 停止上班上課"},
			{"first_seen_at", "timestamp", "", "本站第一次抓到的時間"},
			{"last_seen_at", "timestamp", "", "最後一次還在頁面上的時間"},
		},
		LatestSQL: `
			SELECT DISTINCT ON (s.county_code, s.day)
			       s.county_code, s.day, s.announced_at, s.status, s.level, s.first_seen_at, s.last_seen_at
			  FROM data.work_closure s
			 WHERE ` + countyOnly + ` AND s.day >= (now() AT TIME ZONE 'Asia/Taipei')::date
			 ORDER BY s.county_code, s.day, s.announced_at DESC`,
		HistorySQL: history(`s.county_code, s.day, s.announced_at, s.status, s.level, s.first_seen_at, s.last_seen_at`,
			`data.work_closure s`, countyOnly, `s.announced_at`, `s.county_code || '|' || s.day::text`),
		Months: months(`announced_at`, `data.work_closure`),
	},
	{
		ID:          "shelter",
		Description: "內政部消防署的避難收容處所名冊，is_open 是來源標示「開設」的狀態。名冊沒有時間軸，只有 latest（分頁）。不含管理人與聯絡人資料。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"shelter_id", "string", "", "收容所代碼"},
			{"code", "string", "", "來源的編號"},
			{"name", "string", "", "名稱"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"address", "string", "", "地址"},
			{"disaster_types", "string", "", "適用災害，例如「水災,震災,土石流」"},
			{"indoor", "boolean", "", "室內"},
			{"outdoor", "boolean", "", "室外"},
			{"for_weaker", "boolean", "", "適合避難弱者安置"},
			{"is_open", "boolean", "", "目前開設"},
			lonField, latField,
			{"last_seen_at", "timestamp", "", "最後一次出現在來源的時間"},
		},
		LatestSQL: listLatest(`s.shelter_id, s.code, s.name, s.county_code, s.town, s.address, s.disaster_types, s.indoor, s.outdoor,
			       s.for_weaker, s.is_open, `+lonLat+`, s.last_seen_at`,
			`ref.shelter s`, countyOnly+` AND s.last_seen_at >= now() - interval '3 days'`, `s.shelter_id`),
	},
	{
		ID:          "shelter_report",
		Description: "各縣市政府在災害期間的收容所開設通報：每次開設一列，人數隨通報更新。latest 是目前開設中的收容所。平時沒有資料。歷史資料依開設時間查詢。",
		HasCounty:   true,
		Fields: []Field{
			{"shelter_id", "string", "", "收容所代碼"},
			{"opened_at", "timestamp", "", "開設時間"},
			{"closed_at", "timestamp", "", "撤除時間（開設中為空值）"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"name", "string", "", "名稱"},
			{"refugees", "integer", "人", "目前收容人數"},
			{"refugees_total", "integer", "人", "累計收容人數"},
			{"capacity", "integer", "人", "可收容人數"},
			{"project", "string", "", "災害專案代號"},
			{"reported_at", "timestamp", "", "通報時間"},
			{"first_seen_at", "timestamp", "", "本站第一次抓到的時間"},
			{"last_seen_at", "timestamp", "", "最後一次出現在來源的時間"},
		},
		LatestSQL: `
			SELECT s.shelter_id, s.opened_at, s.closed_at, s.county_code, s.town, s.name, s.refugees, s.refugees_total, s.capacity,
			       s.project, s.reported_at, s.first_seen_at, s.last_seen_at
			  FROM data.shelter_report s
			 WHERE ` + countyOnly + ` AND s.closed_at IS NULL AND s.last_seen_at >= now() - interval '1 hour'
			 ORDER BY s.county_code, s.name`,
		HistorySQL: history(`s.shelter_id, s.opened_at, s.closed_at, s.county_code, s.town, s.name, s.refugees, s.refugees_total, s.capacity,
			       s.project, s.reported_at, s.first_seen_at, s.last_seen_at`,
			`data.shelter_report s`, countyOnly, `s.opened_at`, `s.shelter_id`),
		Months: months(`opened_at`, `data.shelter_report`),
	},
	imageDataset("radar_image", "radar", "中央氣象署的臺灣雷達合成回波圖（每 10 分鐘），本站縮小為網頁用的大小。", false),
	imageDataset("satellite_image", "satellite", "中央氣象署的東亞紅外線彩色衛星雲圖（每 10 分鐘），本站縮小為網頁用的大小。", false),
	imageDataset("radar_layer", "radar_layer", "由中央氣象署雷達回波格點資料上色的透明圖層，可以疊在地圖上（範圍見 west、south、east、north）。", true),
	imageDataset("cloud_layer", "cloud_layer", "NASA GIBS 的向日葵衛星紅外線圖磚，由本站依亮度溫度畫成白色透明的雲，可以疊在地圖上。", true),
	imageDataset("wind_layer", "wind_layer", "中央氣象署 WRF 數值預報的 10 公尺風場，編碼為圖片（紅色、綠色是東西向與南北向風速），可以疊在地圖上做流動動畫。", true),

	{
		ID:          "river_quality",
		Description: "環境部河川水質監測站每次採樣的河川污染指數（RPI）：2.0 以下未（稍）受污染、2.0–3.0 輕度、3.1–6.0 中度、6.0 以上嚴重污染。latest 是各站最近一次的採樣。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"site_id", "string", "", "測站代碼"},
			{"name", "string", "", "測站名稱"},
			countyField,
			{"township", "string", "", "鄉鎮市區"},
			{"basin", "string", "", "流域"},
			{"river", "string", "", "河川"},
			lonField, latField,
		},
		SitesSQL: `
			SELECT s.site_id, s.name, s.county_code, s.township, s.basin, s.river, ` + lonLat + `
			  FROM ref.river_quality_site s WHERE ` + countyCond + ` ORDER BY s.site_id`,
		Fields: []Field{
			{"site_id", "string", "", "測站代碼"},
			{"sampled_at", "timestamp", "", "採樣時間"},
			{"rpi", "number", "", "河川污染指數"},
		},
		LatestSQL: `
			SELECT o.site_id, o.sampled_at, o.rpi::float8
			  FROM data.river_quality_latest o JOIN ref.river_quality_site s USING (site_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.site_id = $2)
			 ORDER BY o.site_id`,
		HistorySQL: history(`o.site_id, o.sampled_at, o.rpi::float8`,
			`data.river_quality o JOIN ref.river_quality_site s USING (site_id)`,
			countyCond+` AND ($2::text IS NULL OR o.site_id = $2)`, `o.sampled_at`, `o.site_id`),
		Months: months(`sampled_at`, `data.river_quality`),
	},
	{
		ID:          "renewable_month",
		Description: "經濟部能源署的每月再生能源發電量，依能源別。latest 是最新一個月。",
		Fields: []Field{
			{"month", "date", "", "月份（該月 1 日）"},
			{"source", "string", "", "hydro 水力、geothermal 地熱、solar 太陽光電、wind_onshore 陸域風力、wind_offshore 離岸風力、biomass_solid 生質能（固態）、biomass_gas 生質能（氣態）、waste 廢棄物"},
			{"gen_mwh", "number", "千度（MWh）", "發電量"},
		},
		LatestSQL: `
			SELECT s.month, s.source, s.gen_mwh::float8 FROM data.renewable_month s
			 WHERE ` + noFilter + ` AND s.month = (SELECT max(month) FROM data.renewable_month)
			 ORDER BY s.source`,
		HistorySQL: history(`s.month, s.source, s.gen_mwh::float8`, `data.renewable_month s`, noFilter, dayStart(`s.month`), `s.source`),
		Months:     `SELECT to_char(month, 'YYYY-MM'), count(*) FROM data.renewable_month GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "waste",
		Description: "環境部各縣市一般廢棄物：產生量＝垃圾＋資源回收＋廚餘，單位公噸。period_type 為 year 是全年、month 是單月。latest 是各自最新的一期。",
		HasCounty:   true,
		Fields: []Field{
			{"period_type", "string", "", "year 全年、month 單月"},
			{"period", "date", "", "期間（該年 1 月 1 日或該月 1 日）"},
			countyField,
			{"total_t", "number", "公噸", "一般廢棄物產生量"},
			{"garbage_t", "number", "公噸", "垃圾量（焚化、掩埋）"},
			{"recycled_t", "number", "公噸", "資源回收量"},
			{"food_t", "number", "公噸", "廚餘回收量"},
			{"per_capita_kg", "number", "公斤", "平均每人每日產生量"},
		},
		LatestSQL: `
			SELECT s.period_type, s.period, s.county_code, s.total_t::float8, s.garbage_t::float8, s.recycled_t::float8,
			       s.food_t::float8, s.per_capita_kg::float8
			  FROM data.waste_county s
			 WHERE ` + countyOnly + ` AND s.period = (SELECT max(period) FROM data.waste_county w WHERE w.period_type = s.period_type)
			 ORDER BY s.period_type, s.county_code`,
		HistorySQL: history(`s.period_type, s.period, s.county_code, s.total_t::float8, s.garbage_t::float8, s.recycled_t::float8,
			       s.food_t::float8, s.per_capita_kg::float8`,
			`data.waste_county s`, countyOnly, dayStart(`s.period`), `s.period_type || '|' || s.county_code`),
		Months: `SELECT to_char(period, 'YYYY-MM'), count(*) FROM data.waste_county GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "power_sales",
		Description: "台灣電力公司各縣市各部門的每月售電量（度）。latest 是最新一個月。",
		HasCounty:   true,
		Fields: []Field{
			{"month", "date", "", "月份（該月 1 日）"},
			countyField,
			{"sector", "string", "", "residential 住宅、service 服務業、agriculture 農林漁牧、industry 工業"},
			{"kwh", "integer", "度", "售電量"},
		},
		LatestSQL: `
			SELECT s.month, s.county_code, s.sector, s.kwh FROM data.power_sales_month s
			 WHERE ` + countyOnly + ` AND s.month = (SELECT max(month) FROM data.power_sales_month)
			 ORDER BY s.county_code, s.sector`,
		HistorySQL: history(`s.month, s.county_code, s.sector, s.kwh`, `data.power_sales_month s`, countyOnly,
			dayStart(`s.month`), `s.county_code || '|' || s.sector`),
		Months: `SELECT to_char(month, 'YYYY-MM'), count(*) FROM data.power_sales_month GROUP BY 1 ORDER BY 1`,
	},

	{
		ID:          "medical_facility",
		Description: "衛生福利部中央健康保險署的健保特約醫事機構（醫學中心、區域醫院、地區醫院、診所、藥局）。名冊沒有時間軸，只有 latest（分頁）。來源沒有座標。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"facility_id", "string", "", "醫事機構代碼"},
			{"name", "string", "", "名稱"},
			{"level", "string", "", "medical_center 醫學中心、regional 區域醫院、district 地區醫院、clinic 診所、pharmacy 藥局"},
			{"kind", "string", "", "醫事機構種類，例如綜合醫院、牙醫一般診所"},
			countyField,
			{"address", "string", "", "地址"},
			{"phone", "string", "", "機構電話"},
			{"departments", "string", "", "診療科別"},
			{"services", "string", "", "服務項目"},
			{"contract_end", "date", "", "終止合約或歇業日期"},
		},
		LatestSQL: listLatest(`s.facility_id, s.name, s.level, s.kind, s.county_code, s.address, s.phone, s.departments, s.services, s.contract_end`,
			`ref.medical_facility s`, countyOnly+` AND s.last_seen_at >= now() - interval '3 days'`, `s.facility_id`),
	},
	{
		ID:          "ltc_facility",
		Description: "衛生福利部的長照服務據點（A 社區整合型服務中心、B 複合型服務中心、C 巷弄長照站），同一個機構的多個服務合併成一列。名冊只有 latest（分頁）。不含負責人資料。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"facility_id", "string", "", "機構代碼"},
			{"name", "string", "", "名稱"},
			{"abc", "string", "", "A、B 或 C 據點"},
			countyField,
			{"town_code", "string", "", "鄉鎮市區代碼"},
			{"address", "string", "", "地址"},
			{"services", "string", "", "特約服務項目（以頓號分隔）"},
			{"beds", "integer", "床", "開放床數（住宿式機構）"},
			{"residents", "integer", "人", "住民人數"},
			{"contract_end", "date", "", "特約到期日"},
			lonField, latField,
		},
		LatestSQL: listLatest(`s.facility_id, s.name, s.abc, s.county_code, s.town_code, s.address, s.services, s.beds, s.residents,
			       s.contract_end, `+lonLat,
			`ref.ltc_facility s`, countyOnly+` AND s.last_seen_at >= now() - interval '40 days'`, `s.facility_id`),
	},
	{
		ID:          "farm_price",
		Description: "農業部的果菜花卉批發市場交易行情：每天各種蔬菜、水果、花卉在全國批發市場的平均價（依交易量加權）與交易量。latest 是最新一個交易日。",
		Fields: []Field{
			{"trade_date", "date", "", "交易日期"},
			{"category", "string", "", "vegetable 蔬菜、fruit 水果、flower 花卉"},
			{"crop", "string", "", "作物名稱（不分品種）"},
			{"avg_price", "number", "元/公斤（花卉為元/把）", "平均價"},
			{"volume", "number", "公斤（花卉為把）", "交易量"},
			{"markets", "integer", "", "有交易的市場數"},
		},
		LatestSQL: `
			SELECT s.trade_date, s.category, s.crop, s.avg_price::float8, s.volume::float8, s.markets FROM data.farm_price_daily s
			 WHERE ` + noFilter + ` AND s.trade_date = (SELECT max(trade_date) FROM data.farm_price_daily)
			 ORDER BY s.category, s.volume DESC`,
		HistorySQL: history(`s.trade_date, s.category, s.crop, s.avg_price::float8, s.volume::float8, s.markets`,
			`data.farm_price_daily s`, noFilter, dayStart(`s.trade_date`), `s.category || '|' || s.crop`),
		Months: `SELECT to_char(trade_date, 'YYYY-MM'), count(*) FROM data.farm_price_daily GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "poultry_price",
		Description: "農業部的家禽交易行情：雞蛋與白肉雞每天的價格（元/台斤）。latest 是最新一天。",
		Fields: []Field{
			{"day", "date", "", "日期"},
			{"egg_farm", "number", "元/台斤", "雞蛋產地價"},
			{"egg_transport", "number", "元/台斤", "雞蛋大運輸價"},
			{"broiler_large", "number", "元/台斤", "白肉雞 2.0 公斤以上"},
			{"broiler_small", "number", "元/台斤", "白肉雞 1.75–1.95 公斤"},
			{"broiler_retail", "number", "元/台斤", "白肉雞門市價（高屏）"},
		},
		LatestSQL: `
			SELECT s.day, s.egg_farm::float8, s.egg_transport::float8, s.broiler_large::float8, s.broiler_small::float8, s.broiler_retail::float8
			  FROM data.poultry_price s WHERE ` + noFilter + ` ORDER BY s.day DESC LIMIT 1`,
		HistorySQL: history(`s.day, s.egg_farm::float8, s.egg_transport::float8, s.broiler_large::float8, s.broiler_small::float8, s.broiler_retail::float8`,
			`data.poultry_price s`, noFilter, dayStart(`s.day`), `''`),
		Months: `SELECT to_char(day, 'YYYY-MM'), count(*) FROM data.poultry_price GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "hog_trade",
		Description: "農業部的毛豬交易行情：每天各肉品市場的成交頭數、規格豬平均價格與重量。latest 是最新一天。",
		Fields: []Field{
			{"day", "date", "", "日期"},
			{"market", "string", "", "肉品市場"},
			{"head", "integer", "頭", "成交頭數"},
			{"price", "number", "元/公斤", "規格豬平均價格"},
			{"weight_kg", "number", "公斤", "規格豬平均重量"},
		},
		LatestSQL: `
			SELECT s.day, s.market, s.head, s.price::float8, s.weight_kg::float8 FROM data.hog_trade s
			 WHERE ` + noFilter + ` AND s.day = (SELECT max(day) FROM data.hog_trade)
			 ORDER BY s.market`,
		HistorySQL: history(`s.day, s.market, s.head, s.price::float8, s.weight_kg::float8`,
			`data.hog_trade s`, noFilter, dayStart(`s.day`), `s.market`),
		Months: `SELECT to_char(day, 'YYYY-MM'), count(*) FROM data.hog_trade GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "food_rejection",
		Description: "衛生福利部食品藥物管理署的邊境查驗不合格輸入食品與食品器具。不含進口商名稱與地址。latest 是最近 30 天公布的。歷史資料依公布日期查詢。",
		Fields: []Field{
			{"rejection_id", "string", "", "本站編號"},
			{"published_on", "date", "", "公布日期"},
			{"received_on", "date", "", "報驗受理日期"},
			{"origin", "string", "", "產地"},
			{"product", "string", "", "品名"},
			{"reason", "string", "", "不合格原因"},
			{"hs_code", "string", "", "貨品分類號列"},
			{"disposal", "string", "", "處置情形，例如退運或銷毀"},
		},
		LatestSQL: `
			SELECT s.rejection_id, s.published_on, s.received_on, s.origin, s.product, s.reason, s.hs_code, s.disposal
			  FROM data.food_rejection s
			 WHERE ` + noFilter + ` AND s.published_on >= (SELECT max(published_on) FROM data.food_rejection) - 30
			 ORDER BY s.published_on DESC, s.rejection_id`,
		HistorySQL: history(`s.rejection_id, s.published_on, s.received_on, s.origin, s.product, s.reason, s.hs_code, s.disposal`,
			`data.food_rejection s`, noFilter, dayStart(`s.published_on`), `s.rejection_id`),
		Months: `SELECT to_char(published_on, 'YYYY-MM'), count(*) FROM data.food_rejection GROUP BY 1 ORDER BY 1`,
	},
	{
		ID: "real_estate",
		Description: "內政部不動產實價登錄（每季公布）：kind 為 sale 買賣、presale 預售屋、rent 租賃。不含門牌、建案名稱與備註原文。special 表示備註有親友、員工等特殊關係。" +
			"latest 是最新一季公布的交易（分頁）。歷史資料依交易日期查詢。",
		HasCounty: true, LatestPaged: true,
		Fields: []Field{
			{"serial", "string", "", "實價登錄編號"},
			{"kind", "string", "", "sale 買賣、presale 預售屋、rent 租賃"},
			{"season", "string", "", "公布的季，例如 115S3（民國 115 年第 3 季）"},
			{"deal_date", "date", "", "交易日期"},
			countyField,
			{"district", "string", "", "鄉鎮市區"},
			{"target", "string", "", "交易標的，例如房地(土地+建物)+車位"},
			{"building_type", "string", "", "建物型態，例如住宅大樓"},
			{"main_use", "string", "", "主要用途，例如住家用"},
			{"total_price", "integer", "元", "總價（租賃為每月租金）"},
			{"unit_price", "number", "元/平方公尺", "單價"},
			{"area_m2", "number", "平方公尺", "建物移轉（租賃）總面積"},
			{"rooms", "integer", "", "房數"},
			{"special", "boolean", "", "特殊關係交易"},
		},
		LatestSQL: listLatest(`s.serial, s.kind, s.season, s.deal_date, s.county_code, s.district, s.target, s.building_type, s.main_use,
			       s.total_price, s.unit_price::float8, s.area_m2::float8, s.rooms, s.special`,
			`data.real_estate_deal s`, countyOnly+` AND s.season = (SELECT max(season) FROM data.real_estate_deal)`, `s.serial`),
		HistorySQL: history(`s.serial, s.kind, s.season, s.deal_date, s.county_code, s.district, s.target, s.building_type, s.main_use,
			       s.total_price, s.unit_price::float8, s.area_m2::float8, s.rooms, s.special`,
			`data.real_estate_deal s`, countyOnly, dayStart(`s.deal_date`), `s.serial`),
		Months: `SELECT to_char(deal_date, 'YYYY-MM'), count(*) FROM data.real_estate_deal GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "dengue_daily",
		Description: "衛生福利部疾病管制署的登革熱確定病例：每個發病日、居住鄉鎮、本土或境外移入一列。latest 是最近 7 天。歷史資料依發病日期查詢。",
		HasCounty:   true,
		Fields: []Field{
			{"onset_date", "date", "", "發病日期"},
			countyField,
			{"town", "string", "", "居住鄉鎮市區"},
			{"imported", "boolean", "", "境外移入（false 為本土）"},
			{"cases", "integer", "例", "確定病例數"},
		},
		LatestSQL: `
			SELECT s.onset_date, s.county_code, s.town, s.imported, s.cases FROM data.dengue_daily s
			 WHERE ` + countyOnly + ` AND s.onset_date >= (SELECT max(onset_date) FROM data.dengue_daily) - 6
			 ORDER BY s.onset_date DESC, s.county_code, s.town`,
		HistorySQL: history(`s.onset_date, s.county_code, s.town, s.imported, s.cases`, `data.dengue_daily s`, countyOnly,
			dayStart(`s.onset_date`), `s.county_code || '|' || s.town || '|' || s.imported::text`),
		Months: `SELECT to_char(onset_date, 'YYYY-MM'), count(*) FROM data.dengue_daily GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "nhi_visit",
		Description: "衛生福利部疾病管制署的健保門診、急診與住院每週就診人次：類流感、腸病毒、新冠，以及全部就診人次（可算出就診比例）。latest 是各疾病最新一週。",
		HasCounty:   true,
		Fields: []Field{
			{"disease", "string", "", "ili 類流感、ev 腸病毒、covid 新冠"},
			{"year", "integer", "", "年"},
			{"week", "integer", "", "週次"},
			{"week_start", "date", "", "該週的星期一"},
			countyField,
			{"visit_type", "string", "", "outpatient 門診、emergency 急診、inpatient 住院"},
			{"visits", "integer", "人次", "該疾病的就診人次"},
			{"total", "integer", "人次", "全部就診人次"},
		},
		LatestSQL: `
			SELECT s.disease, s.year, s.week, s.week_start, s.county_code, s.visit_type, s.visits, s.total FROM data.nhi_visit_week s
			 WHERE ` + countyOnly + ` AND s.week_start = (SELECT max(week_start) FROM data.nhi_visit_week w WHERE w.disease = s.disease)
			 ORDER BY s.disease, s.county_code, s.visit_type`,
		HistorySQL: history(`s.disease, s.year, s.week, s.week_start, s.county_code, s.visit_type, s.visits, s.total`,
			`data.nhi_visit_week s`, countyOnly, dayStart(`s.week_start`), `s.disease || '|' || s.county_code || '|' || s.visit_type`),
		Months: `SELECT to_char(week_start, 'YYYY-MM'), count(*) FROM data.nhi_visit_week GROUP BY 1 ORDER BY 1`,
	},

	{
		ID:          "freeway_traffic",
		Description: "交通部高速公路局的國道即時路況：各路段的旅行速率與旅行時間。latest 是最新一筆（來源每分鐘更新）。歷史資料每 10 分鐘一筆。路段清單在 sites。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"section_id", "string", "", "路段代碼"},
			{"name", "string", "", "路段名稱，例如「國道1號(基隆端至基隆)」"},
			{"road_name", "string", "", "道路名稱"},
			{"direction", "string", "", "方向：N、S、E、W"},
			{"start_name", "string", "", "起點"},
			{"end_name", "string", "", "終點"},
			{"length_km", "number", "公里", "長度"},
			{"speed_limit", "integer", "公里/小時", "速限"},
			countyField,
		},
		SitesSQL: `
			SELECT s.section_id, s.name, s.road_name, s.direction, s.start_name, s.end_name, s.length_km::float8, s.speed_limit, s.county_code
			  FROM ref.freeway_section s WHERE ` + countyCond + ` ORDER BY s.section_id`,
		Fields: []Field{
			{"section_id", "string", "", "路段代碼"},
			{"observed_at", "timestamp", "", "資料時間"},
			{"speed_kmh", "integer", "公里/小時", "旅行速率"},
			{"travel_time_s", "integer", "秒", "旅行時間"},
			{"level", "integer", "", "高速公路局的壅塞分級：1 順暢…5 壅塞，-1 道路封閉"},
		},
		LatestSQL: `
			SELECT o.section_id, o.observed_at, o.speed_kmh, o.travel_time_s, o.level
			  FROM data.freeway_traffic_current o JOIN ref.freeway_section s USING (section_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.section_id = $2)
			 ORDER BY o.section_id`,
		HistorySQL: history(`o.section_id, o.observed_at, o.speed_kmh, o.travel_time_s, o.level`,
			`data.freeway_traffic_history o JOIN ref.freeway_section s USING (section_id)`,
			countyCond+` AND ($2::text IS NULL OR o.section_id = $2)`, `o.observed_at`, `o.section_id`),
		Months: months(`observed_at`, `data.freeway_traffic_history`),
	},
	{
		ID:          "fuel_price",
		Description: "台灣中油的汽柴油參考牌價，每次調價一列（含 1999 年以來的歷史）。latest 是各油品目前的價格。歷史資料依生效時間查詢。",
		Fields: []Field{
			{"product", "string", "", "油品，例如 92無鉛汽油、超級柴油"},
			{"effective_at", "timestamp", "", "生效時間"},
			{"price", "number", "元", "參考牌價"},
			{"unit", "string", "", "單位，例如元/公升"},
		},
		LatestSQL: `
			SELECT DISTINCT ON (s.product) s.product, s.effective_at, s.price::float8, s.unit
			  FROM data.fuel_price s WHERE ` + noFilter + `
			 ORDER BY s.product, s.effective_at DESC`,
		HistorySQL: history(`s.product, s.effective_at, s.price::float8, s.unit`, `data.fuel_price s`, noFilter, `s.effective_at`, `s.product`),
		Months:     months(`effective_at`, `data.fuel_price`),
	},
	{
		ID:          "fuel_station",
		Description: "台灣中油的加油站名冊與服務。名冊只有 latest（分頁）。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"station_id", "string", "", "加油站代碼"},
			{"name", "string", "", "名稱"},
			{"kind", "string", "", "直營站、加盟站"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"address", "string", "", "地址"},
			{"open_hours", "string", "", "營業時間"},
			{"operating", "boolean", "", "營業中"},
			{"self_service", "boolean", "", "自助加油"},
			{"freeway", "boolean", "", "國道服務區"},
			{"has_98", "boolean", "", "供應 98 無鉛汽油"},
			{"has_diesel", "boolean", "", "供應柴油"},
			lonField, latField,
		},
		LatestSQL: listLatest(`s.station_id, s.name, s.kind, s.county_code, s.town, s.address, s.open_hours, s.operating, s.self_service,
			       s.freeway, s.has_98, s.has_diesel, `+lonLat,
			`ref.fuel_station s`, countyOnly+` AND s.last_seen_at >= now() - interval '3 days'`, `s.station_id`),
	},
	{
		ID: "rail_alert",
		Description: "交通部 TDX 的臺鐵（TRA）與高鐵（THSR）營運通阻。latest 是目前生效中的通阻。歷史資料依本站第一次抓到的時間查詢。" +
			"本站約每 2 小時更新一次（TDX 的每月額度）。",
		Fields: []Field{
			{"operator", "string", "", "TRA 臺鐵、THSR 高鐵"},
			{"alert_id", "string", "", "通阻代碼"},
			{"title", "string", "", "標題"},
			{"description", "string", "", "內容"},
			{"reason", "string", "", "原因"},
			{"status", "integer", "", "TDX 的狀態代碼"},
			{"level", "integer", "", "TDX 的影響等級"},
			{"lines", "string", "", "影響的路線（以頓號分隔）"},
			{"stations", "string", "", "影響的車站（以頓號分隔）"},
			{"starts_at", "timestamp", "", "開始時間"},
			{"ends_at", "timestamp", "", "結束時間"},
			{"published_at", "timestamp", "", "發布時間"},
			{"updated_at", "timestamp", "", "更新時間"},
			{"first_seen_at", "timestamp", "", "本站第一次抓到的時間"},
			{"last_seen_at", "timestamp", "", "最後一次出現在來源的時間"},
		},
		LatestSQL: `
			SELECT a.operator, a.alert_id, a.title, a.description, a.reason, a.status, a.level,
			       array_to_string(a.lines, '、'), array_to_string(a.stations, '、'), a.starts_at, a.ends_at,
			       a.published_at, a.updated_at, a.first_seen_at, a.last_seen_at
			  FROM data.rail_alert_active a WHERE ` + noFilter + `
			 ORDER BY a.operator DESC, a.published_at DESC NULLS LAST`,
		HistorySQL: history(`a.operator, a.alert_id, a.title, a.description, a.reason, a.status, a.level,
			       array_to_string(a.lines, '、'), array_to_string(a.stations, '、'), a.starts_at, a.ends_at,
			       a.published_at, a.updated_at, a.first_seen_at, a.last_seen_at`,
			`data.rail_alert a`, noFilter, `a.first_seen_at`, `a.operator || '|' || a.alert_id`),
		Months: months(`first_seen_at`, `data.rail_alert`),
	},
	{
		ID: "tra_live",
		Description: "交通部 TDX 的臺鐵列車即時位置與誤點。latest 是目前行駛中的每一列車（可以用 site 指定車站）。" +
			"history 是每次更新的誤點摘要（行駛中列車數、誤點 5 分鐘以上的列車數、平均與最大誤點），site 只用於 latest。車站清單在 sites。本站約每小時更新一次。",
		HasSite: true,
		SiteFields: []Field{
			{"station_id", "string", "", "車站代碼"},
			{"name", "string", "", "站名"},
			countyField,
			{"address", "string", "", "地址"},
			lonField, latField,
		},
		SitesSQL: `
			SELECT s.station_id, s.name, s.county_code, s.address, ` + lonLat + `
			  FROM ref.tra_station s WHERE ($1::text IS NULL OR s.county_code = $1) ORDER BY s.station_id`,
		LatestFields: []Field{
			{"train_no", "string", "", "車次"},
			{"train_type", "string", "", "車種，例如區間、自強"},
			{"station_id", "string", "", "所在或剛離開的車站代碼"},
			{"station_name", "string", "", "車站名稱"},
			{"station_status", "integer", "", "0 進站中、1 在站上、2 已離站"},
			{"delay_min", "integer", "分鐘", "誤點分鐘"},
			{"updated_at", "timestamp", "", "來源的更新時間"},
		},
		LatestSQL: `
			SELECT t.train_no, t.train_type, t.station_id, t.station_name, t.station_status, t.delay_min, t.updated_at
			  FROM data.tra_train_live t
			 WHERE $1::text IS NULL AND ($2::text IS NULL OR t.station_id = $2)
			 ORDER BY t.delay_min DESC, t.train_no`,
		Fields: []Field{
			{"observed_at", "timestamp", "", "更新時間"},
			{"running", "integer", "列", "行駛中的列車"},
			{"delayed", "integer", "列", "誤點 5 分鐘以上的列車"},
			{"avg_delay_min", "number", "分鐘", "平均誤點"},
			{"max_delay_min", "integer", "分鐘", "最大誤點"},
		},
		HistorySQL: history(`h.observed_at, h.running, h.delayed, h.avg_delay_min::float8, h.max_delay_min`,
			`data.tra_delay_history h`, noFilter, `h.observed_at`, `''`),
		Months: months(`observed_at`, `data.tra_delay_history`),
	},
	{
		ID: "bike_availability",
		Description: "交通部 TDX 的公共自行車（YouBike）即時車位，15 個縣市。latest 是每一站目前的可借車輛與可還空位（分頁）。" +
			"history 是各縣市每次更新的摘要（只算正常營運的站），site 只用於 latest。站點清單在 sites。本站每天 8:12、20:12 更新。",
		HasCounty: true, HasSite: true, LatestPaged: true,
		SiteFields: []Field{
			{"station_uid", "string", "", "站點代碼"},
			{"city", "string", "", "TDX 的縣市名稱，例如 Taipei"},
			countyField,
			{"name", "string", "", "站名"},
			{"address", "string", "", "地址"},
			{"capacity", "integer", "", "車柱數"},
			{"service_type", "integer", "", "1 YouBike 1.0、2 YouBike 2.0"},
			lonField, latField,
		},
		SitesSQL: `
			SELECT s.station_uid, s.city, s.county_code, s.name, s.address, s.capacity, s.service_type, ` + lonLat + `
			  FROM ref.bike_station s WHERE ` + countyCond + ` ORDER BY s.station_uid`,
		LatestFields: []Field{
			{"station_uid", "string", "", "站點代碼"},
			{"status", "integer", "", "0 停止營運、1 正常、2 暫停營運"},
			{"bikes", "integer", "輛", "可借車輛"},
			{"docks", "integer", "", "可還空位"},
			{"ebikes", "integer", "輛", "其中電輔車"},
			{"updated_at", "timestamp", "", "來源的更新時間"},
			{"fetched_at", "timestamp", "", "本站抓取的時間"},
		},
		LatestSQL: listLatest(`a.station_uid, a.status, a.bikes, a.docks, a.ebikes, a.updated_at, a.fetched_at`,
			`data.bike_availability_current a JOIN ref.bike_station s USING (station_uid)`,
			countyCond+` AND ($2::text IS NULL OR a.station_uid = $2)`, `a.station_uid`),
		Fields: []Field{
			countyField,
			{"observed_at", "timestamp", "", "更新時間"},
			{"stations", "integer", "站", "正常營運的站數"},
			{"bikes", "integer", "輛", "可借車輛"},
			{"docks", "integer", "", "可還空位"},
			{"empty_stations", "integer", "站", "沒有車可借的站"},
			{"full_stations", "integer", "站", "沒有空位可還的站"},
		},
		HistorySQL: history(`s.county_code, s.observed_at, s.stations, s.bikes, s.docks, s.empty_stations, s.full_stations`,
			`data.bike_county_history s`, countyOnly, `s.observed_at`, `s.county_code::text`),
		Months: months(`observed_at`, `data.bike_county_history`),
	},
	{
		ID: "ev_live",
		Description: "交通部 TDX 的電動車充電站與充電槍即時狀態。latest 是每一站目前的空閒、使用中與故障的充電槍數。" +
			"history 是各縣市每次更新的摘要，site 只用於 latest。充電站清單（含充電槍規格與費率）在 sites。本站每天中午更新。",
		HasCounty: true, HasSite: true,
		SiteFields: []Field{
			{"station_id", "string", "", "充電站代碼"},
			{"name", "string", "", "名稱"},
			countyField,
			{"operation_type", "integer", "", "TDX 的營運類型代碼"},
			{"spaces", "integer", "", "車位數"},
			{"charging_points", "integer", "", "充電樁數"},
			{"ac_connectors", "integer", "", "交流充電槍數"},
			{"dc_connectors", "integer", "", "直流快充槍數"},
			{"connector_types", "string", "", "充電槍規格（以逗號分隔），例如 J1772、CCS2"},
			{"service_time", "string", "", "服務時間"},
			{"charging_rate", "string", "", "充電費率"},
			{"parking_rate", "string", "", "停車費率"},
			lonField, latField,
		},
		SitesSQL: `
			SELECT s.station_id, s.name, s.county_code, s.operation_type, s.spaces, s.charging_points, s.ac_connectors, s.dc_connectors,
			       array_to_string(s.connector_types, ','), s.service_time, s.charging_rate, s.parking_rate, ` + lonLat + `
			  FROM ref.ev_station s WHERE ` + countyCond + ` ORDER BY s.station_id`,
		LatestFields: []Field{
			{"station_id", "string", "", "充電站代碼"},
			{"free", "integer", "", "空閒的充電槍"},
			{"in_use", "integer", "", "使用中的充電槍"},
			{"broken", "integer", "", "故障的充電槍"},
			{"updated_at", "timestamp", "", "來源的更新時間"},
			{"fetched_at", "timestamp", "", "本站抓取的時間"},
		},
		LatestSQL: `
			SELECT l.station_id, l.free, l.in_use, l.broken, l.updated_at, l.fetched_at
			  FROM data.ev_station_live l JOIN ref.ev_station s USING (station_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR l.station_id = $2)
			 ORDER BY l.station_id`,
		Fields: []Field{
			countyField,
			{"observed_at", "timestamp", "", "更新時間"},
			{"free", "integer", "", "空閒的充電槍"},
			{"in_use", "integer", "", "使用中的充電槍"},
			{"broken", "integer", "", "故障的充電槍"},
		},
		HistorySQL: history(`s.county_code, s.observed_at, s.free, s.in_use, s.broken`,
			`data.ev_county_history s`, countyOnly, `s.observed_at`, `s.county_code::text`),
		Months: months(`observed_at`, `data.ev_county_history`),
	},
	{
		ID: "flight",
		Description: "交通部 TDX 的國內各機場定期航班即時資訊（出發與抵達）。latest 是今天的航班（分頁）。歷史資料依表定時間查詢。" +
			"機場清單（國內外）在 sites，site 是國內的機場代碼。本站約每 3 小時更新一次。",
		HasCounty: true, HasSite: true, LatestPaged: true,
		SiteFields: []Field{
			{"airport_id", "string", "", "機場代碼（IATA，例如 TPE）"},
			{"name", "string", "", "機場名稱"},
			{"city", "string", "", "城市"},
			{"country", "string", "", "國家"},
			countyField, lonField, latField,
		},
		SitesSQL: `
			SELECT s.airport_id, s.name, s.city, s.country, s.county_code, ` + lonLat + `
			  FROM ref.airport s WHERE ` + countyCond + ` ORDER BY s.airport_id`,
		Fields: []Field{
			{"direction", "string", "", "dep 出發、arr 抵達"},
			{"airport_id", "string", "", "國內的機場代碼"},
			{"airline_id", "string", "", "航空公司代碼"},
			{"airline_name", "string", "", "航空公司名稱"},
			{"flight_no", "string", "", "航班號碼"},
			{"scheduled_at", "timestamp", "", "表定時間"},
			{"flight_date", "date", "", "航班日期"},
			{"other_airport", "string", "", "出發航班的目的地、抵達航班的出發地"},
			{"estimated_at", "timestamp", "", "預計時間"},
			{"actual_at", "timestamp", "", "實際時間"},
			{"status", "string", "", "on_time 準時、delayed 延誤、cancelled 取消、changed 時間更改、done 已起降、extra 加班、other 其他"},
			{"remark", "string", "", "備註"},
			{"cargo", "boolean", "", "貨機"},
			{"last_seen_at", "timestamp", "", "最後一次出現在來源的時間"},
		},
		LatestSQL: listLatest(`f.direction, f.airport_id, f.airline_id, al.name, f.flight_no, f.scheduled_at, f.flight_date, f.other_airport,
			       f.estimated_at, f.actual_at, f.status, f.remark, f.cargo, f.last_seen_at`,
			`data.flight f JOIN ref.airport s ON s.airport_id = f.airport_id LEFT JOIN ref.airline al ON al.airline_id = f.airline_id`,
			countyCond+` AND ($2::text IS NULL OR f.airport_id = $2) AND f.flight_date = (now() AT TIME ZONE 'Asia/Taipei')::date`,
			`to_char(f.scheduled_at AT TIME ZONE 'UTC', 'YYYYMMDDHH24MI') || '|' || f.direction || '|' || f.airport_id || '|' || f.airline_id || '|' || f.flight_no`),
		HistorySQL: history(`f.direction, f.airport_id, f.airline_id, al.name, f.flight_no, f.scheduled_at, f.flight_date, f.other_airport,
			       f.estimated_at, f.actual_at, f.status, f.remark, f.cargo, f.last_seen_at`,
			`data.flight f JOIN ref.airport s ON s.airport_id = f.airport_id LEFT JOIN ref.airline al ON al.airline_id = f.airline_id`,
			countyCond+` AND ($2::text IS NULL OR f.airport_id = $2)`, `f.scheduled_at`,
			`f.direction || '|' || f.airport_id || '|' || f.airline_id || '|' || f.flight_no`),
		Months: months(`scheduled_at`, `data.flight`),
	},
	{
		ID:          "traffic_accident",
		Description: "內政部警政署的 A1 類道路交通事故（24 小時內死亡），大約每月公布。來源一位當事人一列，本站以發生時間與地點合併成一件。不含當事人的性別、年齡等個人資料。latest 是最新一個月。歷史資料依發生時間查詢。",
		HasCounty:   true,
		Fields: []Field{
			{"accident_id", "string", "", "本站編號（發生時間與地點的雜湊）"},
			{"occurred_at", "timestamp", "", "發生時間"},
			countyField,
			{"police", "string", "", "處理的警察局"},
			{"place", "string", "", "發生地點"},
			{"weather", "string", "", "天候"},
			{"light", "string", "", "光線"},
			{"road_class", "string", "", "道路類別，例如市區道路、省道"},
			{"speed_limit", "integer", "公里/小時", "速限"},
			{"accident_type", "string", "", "事故類型，例如車與車、人與車"},
			{"cause", "string", "", "主要肇因"},
			{"vehicle", "string", "", "第一當事者的車種"},
			{"deaths", "integer", "人", "死亡人數"},
			{"injuries", "integer", "人", "受傷人數"},
			lonField, latField,
		},
		LatestSQL: `
			SELECT s.accident_id, s.occurred_at, s.county_code, s.police, s.place, s.weather, s.light, s.road_class, s.speed_limit,
			       s.accident_type, s.cause, s.vehicle, s.deaths, s.injuries, ` + lonLat + `
			  FROM data.traffic_accident s
			 WHERE ` + countyOnly + ` AND s.occurred_at >= date_trunc('month', (SELECT max(occurred_at) FROM data.traffic_accident) AT TIME ZONE 'Asia/Taipei') AT TIME ZONE 'Asia/Taipei'
			 ORDER BY s.occurred_at DESC`,
		HistorySQL: history(`s.accident_id, s.occurred_at, s.county_code, s.police, s.place, s.weather, s.light, s.road_class, s.speed_limit,
			       s.accident_type, s.cause, s.vehicle, s.deaths, s.injuries, `+lonLat,
			`data.traffic_accident s`, countyOnly, `s.occurred_at`, `s.accident_id`),
		Months: months(`occurred_at`, `data.traffic_accident`),
	},

	{
		ID:          "population_year",
		Description: "內政部戶政司的各縣市年底戶數與人口數（由各村里加總）。latest 是最新一年。歷史資料的時間是該年 12 月 31 日。",
		HasCounty:   true,
		Fields: []Field{
			{"year", "integer", "", "西元年"},
			countyField,
			{"households", "integer", "戶", "戶數"},
			{"population", "integer", "人", "人口數"},
			{"male", "integer", "人", "男性"},
			{"female", "integer", "人", "女性"},
		},
		LatestSQL: `
			SELECT s.year, s.county_code, s.households, s.population, s.male, s.female FROM data.population_year s
			 WHERE ` + countyOnly + ` AND s.year = (SELECT max(year) FROM data.population_year)
			 ORDER BY s.county_code`,
		HistorySQL: history(`s.year, s.county_code, s.households, s.population, s.male, s.female`,
			`data.population_year s`, countyOnly, yearEnd(`s.year`), `s.county_code::text`),
		Months: `SELECT year::text || '-12', count(*) FROM data.population_year GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "vital_month",
		Description: "內政部戶政司的各縣市每月出生數、死亡數、結婚與離婚對數（由各村里加總）。latest 是最新一個月。",
		HasCounty:   true,
		Fields: []Field{
			{"month", "date", "", "月份（該月 1 日）"},
			countyField,
			{"births", "integer", "人", "出生數"},
			{"deaths", "integer", "人", "死亡數"},
			{"marriages", "integer", "對", "結婚對數"},
			{"divorces", "integer", "對", "離婚對數"},
		},
		LatestSQL: `
			SELECT s.month, s.county_code, s.births, s.deaths, s.marriages, s.divorces FROM data.vital_month s
			 WHERE ` + countyOnly + ` AND s.month = (SELECT max(month) FROM data.vital_month)
			 ORDER BY s.county_code`,
		HistorySQL: history(`s.month, s.county_code, s.births, s.deaths, s.marriages, s.divorces`,
			`data.vital_month s`, countyOnly, dayStart(`s.month`), `s.county_code::text`),
		Months: `SELECT to_char(month, 'YYYY-MM'), count(*) FROM data.vital_month GROUP BY 1 ORDER BY 1`,
	},
	{
		ID: "econ_indicator",
		Description: "行政院主計總處與國家發展委員會的每月經濟指標，一個指標一列：例如 cpi:總指數（消費者物價指數）、cpi_yoy:食物類（年增率）、" +
			"unemployment:總計（失業率）、signal_score（景氣對策信號綜合分數）。latest 是每個指標最新的一個月。",
		Fields: []Field{
			{"series", "string", "", "指標代碼，例如 cpi:總指數"},
			{"month", "date", "", "月份（該月 1 日）"},
			{"value", "number", "", "數值（單位依指標而定：指數、%、分）"},
		},
		LatestSQL: `
			SELECT DISTINCT ON (s.series) s.series, s.month, s.value::float8 FROM data.econ_month s
			 WHERE ` + noFilter + `
			 ORDER BY s.series, s.month DESC`,
		HistorySQL: history(`s.series, s.month, s.value::float8`, `data.econ_month s`, noFilter, dayStart(`s.month`), `s.series`),
		Months:     `SELECT to_char(month, 'YYYY-MM'), count(*) FROM data.econ_month GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "income_town",
		Description: "財政部財政資訊中心的綜合所得稅所得總額，每個年度各鄉鎮市區一列（金額單位：千元）。latest 是最新的年度。歷史資料的時間是該年 12 月 31 日。",
		HasCounty:   true,
		Fields: []Field{
			{"year", "integer", "", "所得年度（西元）"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"households", "integer", "", "納稅單位"},
			{"income_k", "integer", "千元", "綜合所得總額"},
			{"mean_k", "integer", "千元", "平均數"},
			{"median_k", "integer", "千元", "中位數"},
		},
		LatestSQL: `
			SELECT s.year, s.county_code, s.town, s.households, s.income_k, s.mean_k, s.median_k FROM data.income_town s
			 WHERE ` + countyOnly + ` AND s.year = (SELECT max(year) FROM data.income_town)
			 ORDER BY s.county_code, s.town`,
		HistorySQL: history(`s.year, s.county_code, s.town, s.households, s.income_k, s.mean_k, s.median_k`,
			`data.income_town s`, countyOnly, yearEnd(`s.year`), `s.county_code || '|' || s.town`),
		Months: `SELECT year::text || '-12', count(*) FROM data.income_town GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "school",
		Description: "教育部統計處的各級學校名錄（國小、國中、高級中等學校、大專校院），只有最新學年度。名冊只有 latest（分頁）。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"school_id", "string", "", "本站代碼（學制:教育部學校代碼）"},
			{"level", "string", "", "elementary 國小、junior 國中、senior 高級中等學校、university 大專校院"},
			{"code", "string", "", "教育部學校代碼"},
			{"name", "string", "", "校名"},
			{"is_public", "boolean", "", "公立"},
			countyField,
			{"address", "string", "", "地址"},
			{"school_year", "integer", "", "民國學年度"},
		},
		LatestSQL: listLatest(`s.school_id, s.level, s.code, s.name, s.is_public, s.county_code, s.address, s.school_year`,
			`ref.school s`, countyOnly, `s.school_id`),
	},
	{
		ID:          "childcare_center",
		Description: "衛生福利部社會福利機構名冊中的托嬰中心。名冊只有 latest（分頁）。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"facility_id", "string", "", "社福機構代碼"},
			{"name", "string", "", "名稱"},
			countyField,
			{"address", "string", "", "地址"},
			lonField, latField,
		},
		LatestSQL: listLatest(`s.facility_id, s.name, s.county_code, s.address, `+lonLat,
			`ref.childcare_center s`, countyOnly+` AND s.last_seen_at >= now() - interval '70 days'`, `s.facility_id`),
	},
	{
		ID:          "tourism_attraction",
		Description: "交通部觀光署的觀光景點。名冊只有 latest（分頁）。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"attraction_id", "string", "", "景點代碼"},
			{"name", "string", "", "名稱"},
			{"classes", "string", "", "觀光署的景點分類代碼（以逗號分隔）"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"address", "string", "", "地址"},
			{"description", "string", "", "簡介（前 200 字）"},
			{"free", "boolean", "", "免費參觀"},
			{"status", "integer", "", "觀光署的服務狀態代碼，1 是正常開放"},
			lonField, latField,
		},
		LatestSQL: listLatest(`s.attraction_id, s.name, array_to_string(s.classes, ','), s.county_code, s.town, s.address, s.description,
			       s.free, s.status, `+lonLat,
			`ref.tourism_attraction s`, countyOnly+` AND s.last_seen_at >= now() - interval '3 days'`, `s.attraction_id`),
	},
	{
		ID:          "tourism_hotel",
		Description: "交通部觀光署的合法旅宿：國際觀光旅館、一般觀光旅館、一般旅館與民宿。名冊只有 latest（分頁）。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"hotel_id", "string", "", "旅宿代碼"},
			{"name", "string", "", "名稱"},
			{"class", "integer", "", "1 國際觀光旅館、2 一般觀光旅館、3 一般旅館、4 民宿"},
			{"stars", "integer", "", "星級"},
			{"rooms", "integer", "間", "房間數"},
			{"capacity", "integer", "人", "可住人數"},
			{"lowest_price", "integer", "元", "最低房價"},
			{"highest_price", "integer", "元", "最高房價"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"address", "string", "", "地址"},
			lonField, latField,
		},
		LatestSQL: listLatest(`s.hotel_id, s.name, s.class, s.stars, s.rooms, s.capacity, s.lowest_price, s.highest_price, s.county_code,
			       s.town, s.address, `+lonLat,
			`ref.tourism_hotel s`, countyOnly+` AND s.last_seen_at >= now() - interval '3 days'`, `s.hotel_id`),
	},
	{
		ID:          "shelter_animal",
		Description: "農業部的全國公立動物收容所開放認養動物（每 6 小時整批更新）。名冊只有 latest（分頁）。不含照片。",
		HasCounty:   true, LatestPaged: true,
		Fields: []Field{
			{"animal_id", "integer", "", "動物編號"},
			{"kind", "string", "", "狗、貓、其他"},
			{"variety", "string", "", "品種"},
			{"sex", "string", "", "M 公、F 母、N 未輸入"},
			{"age", "string", "", "CHILD 幼年、ADULT 成年"},
			{"body_type", "string", "", "SMALL、MEDIUM、BIG"},
			{"colour", "string", "", "毛色"},
			{"sterilized", "boolean", "", "已絕育"},
			{"shelter", "string", "", "收容所"},
			countyField,
			{"open_date", "date", "", "開放認養日期"},
			{"fetched_at", "timestamp", "", "本站抓取的時間"},
		},
		LatestSQL: listLatest(`s.animal_id, s.kind, s.variety, s.sex, s.age, s.body_type, s.colour, s.sterilized, s.shelter, s.county_code,
			       s.open_date, s.fetched_at`,
			`data.shelter_animal s`, countyOnly, `lpad(s.animal_id::text, 12, '0')`),
	},
	{
		ID:          "shelter_stats",
		Description: "農業部的全國公立動物收容所收容處理情形：各縣市每月的收容、認領養、人道處理與所內死亡。latest 是最新一個月。",
		HasCounty:   true,
		Fields: []Field{
			countyField,
			{"month", "date", "", "月份（該月 1 日）"},
			{"accepted", "integer", "隻", "收容"},
			{"adopted", "integer", "隻", "認領養"},
			{"euthanized", "integer", "隻", "人道處理"},
			{"died", "integer", "隻", "所內死亡"},
		},
		LatestSQL: `
			SELECT s.county_code, s.month, s.accepted, s.adopted, s.euthanized, s.died FROM data.shelter_month s
			 WHERE ` + countyOnly + ` AND s.month = (SELECT max(month) FROM data.shelter_month)
			 ORDER BY s.county_code`,
		HistorySQL: history(`s.county_code, s.month, s.accepted, s.adopted, s.euthanized, s.died`,
			`data.shelter_month s`, countyOnly, dayStart(`s.month`), `s.county_code::text`),
		Months: `SELECT to_char(month, 'YYYY-MM'), count(*) FROM data.shelter_month GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "fire_incident",
		Description: "內政部消防署的火災事件，每一件一列（每年公布）。地點只到鄉鎮市區。latest 是資料中最近 30 天的案件。歷史資料依報案時間查詢。",
		HasCounty:   true,
		Fields: []Field{
			{"case_id", "string", "", "火災案件編號"},
			{"reported_at", "timestamp", "", "報案時間"},
			{"arrived_at", "timestamp", "", "消防人員到達時間"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"cause", "string", "", "起火原因，例如電氣因素"},
			{"deaths", "integer", "人", "死亡人數"},
			{"injuries", "integer", "人", "受傷人數"},
		},
		LatestSQL: `
			SELECT s.case_id, s.reported_at, s.arrived_at, s.county_code, s.town, s.cause, s.deaths, s.injuries FROM data.fire_incident s
			 WHERE ` + countyOnly + ` AND s.reported_at >= (SELECT max(reported_at) FROM data.fire_incident) - interval '30 days'
			 ORDER BY s.reported_at DESC`,
		HistorySQL: history(`s.case_id, s.reported_at, s.arrived_at, s.county_code, s.town, s.cause, s.deaths, s.injuries`,
			`data.fire_incident s`, countyOnly, `s.reported_at`, `s.case_id`),
		Months: months(`reported_at`, `data.fire_incident`),
	},
}
