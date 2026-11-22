package opendata

type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string、integer、number、boolean、timestamp、date
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description"`
}

type Dataset struct {
	ID           string
	Description  string
	SiteFields   []Field
	SitesSQL     string // $1 county (or NULL)
	Fields       []Field
	LatestFields []Field
	LatestSQL    string // $1 county, $2 site
	LatestPaged  bool
	HistorySQL   string
	HasCounty    bool
	HasSite      bool
	Months       string
}

func (d *Dataset) LatestRowFields() []Field {
	if d.LatestFields != nil {
		return d.LatestFields
	}
	return d.Fields
}

func (d *Dataset) HasHistory() bool { return d.HistorySQL != "" }

type Topic struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var Topics = []Topic{
	{"weather", "天氣與防災"},
	{"environment", "環境"},
	{"living", "民生"},
	{"transport", "交通"},
	{"society", "人口與社會"},
}

var topicOf = map[string]string{
	"aqi_obs": "environment", "uv_obs": "environment", "aqi_forecast": "environment",
	"reservoir_obs": "environment", "river_level": "environment",
	"water_outage": "living", "water_outage_taipei": "living", "power_outage": "living",
	"typhoon_track": "weather", "typhoon_warning": "weather", "weather_warning": "weather",
	"rain_obs": "weather", "weather_obs": "weather", "county_forecast": "weather",
	"earthquake": "weather", "earthquake_local": "weather",
	"power_gen": "living", "power_region": "living", "power_load": "living", "power_peak_daily": "living",
	"power_supply_daily": "living", "power_supply_forecast": "living",
}

func TopicOf(id string) string {
	if t, ok := topicOf[id]; ok {
		return t
	}
	return "living"
}

var Datasets = append(append([]Dataset{}, coreDatasets...), moreDatasets...)

const (
	typhoonFixCols = `f.typhoon_id, t.name_zh, t.name_en, t.ty_no, f.kind, f.issued_at, f.forecast_hour, f.valid_at,
	         ST_X(f.geom), ST_Y(f.geom), f.pressure_hpa, f.max_wind_ms, f.max_gust_ms, f.moving_speed_kmh, f.moving_dir,
	         f.radius7_km, f.radius10_km, f.radius70_km, f.note`
	rainCols = `o.today_mm::float8, o.past10min_mm::float8, o.past1hr_mm::float8, o.past3hr_mm::float8, o.past6hr_mm::float8,
	         o.past12hr_mm::float8, o.past24hr_mm::float8, o.past2days_mm::float8, o.past3days_mm::float8`
	obsCols = `o.weather, o.temp_c::float8, o.humidity_pct::float8, o.pressure_hpa::float8, o.wind_dir_deg::float8,
	         o.wind_ms::float8, o.gust_ms::float8, o.high_c::float8, o.low_c::float8`
)

const (
	countyCond = `($1::text IS NULL OR s.county_code = $1)`
	pageCond   = `($5::timestamptz IS NULL OR (cursor_t, cursor_k) > ($5, $6))`
)

var countyField = Field{"county_code", "string", "", "縣市代碼（內政部，例如 63000）。尚未標註時為空值"}

var coreDatasets = []Dataset{
	{
		ID:          "aqi_obs",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.aqi_obs_history GROUP BY 1 ORDER BY 1`,
		Description: "環境部空氣品質監測站的每小時觀測值，含空氣品質指標與各污染物濃度。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"site_id", "integer", "", "測站編號（環境部）"},
			{"name", "string", "", "測站名稱"},
			countyField,
			{"lon", "number", "度", "經度（WGS 84）"},
			{"lat", "number", "度", "緯度（WGS 84）"},
		},
		SitesSQL: `
			SELECT s.site_id, s.name, s.county_code, ST_X(s.geom), ST_Y(s.geom)
			  FROM ref.aqi_site s WHERE ` + countyCond + ` ORDER BY s.site_id`,
		Fields: []Field{
			{"site_id", "integer", "", "測站編號"},
			{"observed_at", "timestamp", "", "觀測時間（臺灣時間）"},
			{"aqi", "integer", "", "空氣品質指標 AQI"},
			{"status", "string", "", "狀態（良好、普通…）"},
			{"pollutant", "string", "", "指標污染物"},
			{"pm25", "number", "μg/m³", "細懸浮微粒 PM2.5 濃度"},
			{"pm25_avg", "number", "μg/m³", "PM2.5 移動平均"},
			{"pm10", "number", "μg/m³", "懸浮微粒 PM10 濃度"},
			{"pm10_avg", "number", "μg/m³", "PM10 移動平均"},
			{"o3", "number", "ppb", "臭氧"},
			{"o3_8hr", "number", "ppb", "臭氧 8 小時移動平均"},
			{"co", "number", "ppm", "一氧化碳"},
			{"co_8hr", "number", "ppm", "一氧化碳 8 小時移動平均"},
			{"so2", "number", "ppb", "二氧化硫"},
			{"so2_avg", "number", "ppb", "二氧化硫移動平均"},
			{"no2", "number", "ppb", "二氧化氮"},
			{"nox", "number", "ppb", "氮氧化物"},
			{"no", "number", "ppb", "一氧化氮"},
			{"wind_speed", "number", "m/s", "風速"},
			{"wind_dir", "number", "度", "風向"},
		},
		LatestSQL: `
			SELECT o.site_id, o.observed_at, o.aqi, o.status, o.pollutant,
			       o.pm25_ug_m3::float8, o.pm25_avg_ug_m3::float8, o.pm10_ug_m3::float8, o.pm10_avg_ug_m3::float8,
			       o.o3_ppb::float8, o.o3_8hr_ppb::float8, o.co_ppm::float8, o.co_8hr_ppm::float8,
			       o.so2_ppb::float8, o.so2_avg_ppb::float8, o.no2_ppb::float8, o.nox_ppb::float8, o.no_ppb::float8,
			       o.wind_speed_m_s::float8, o.wind_dir_deg::float8
			  FROM data.aqi_obs_current o JOIN ref.aqi_site s USING (site_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.site_id::text = $2)
			 ORDER BY o.site_id`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT o.site_id, o.observed_at, o.aqi, o.status, o.pollutant,
			         o.pm25_ug_m3::float8, o.pm25_avg_ug_m3::float8, o.pm10_ug_m3::float8, o.pm10_avg_ug_m3::float8,
			         o.o3_ppb::float8, o.o3_8hr_ppb::float8, o.co_ppm::float8, o.co_8hr_ppm::float8,
			         o.so2_ppb::float8, o.so2_avg_ppb::float8, o.no2_ppb::float8, o.nox_ppb::float8, o.no_ppb::float8,
			         o.wind_speed_m_s::float8, o.wind_dir_deg::float8,
			         o.observed_at AS cursor_t, lpad(o.site_id::text, 10, '0') AS cursor_k
			    FROM data.aqi_obs_history o JOIN ref.aqi_site s USING (site_id)
			   WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.site_id::text = $2)
			     AND o.observed_at >= $3 AND o.observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "uv_obs",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.uv_obs_history GROUP BY 1 ORDER BY 1`,
		Description: "環境部整合中央氣象署的紫外線測站，每小時的紫外線指數（UVI）。夜間為 0。儀器異常時為空值。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"site_id", "integer", "", "測站編號（本站編號，來源沒有測站代碼）"},
			{"name", "string", "", "測站名稱"},
			countyField,
			{"agency", "string", "", "測站所屬機關"},
			{"lon", "number", "度", "經度（WGS 84）"},
			{"lat", "number", "度", "緯度（WGS 84）"},
		},
		SitesSQL: `
			SELECT s.site_id, s.name, s.county_code, s.agency, ST_X(s.geom), ST_Y(s.geom)
			  FROM ref.uv_site s WHERE ` + countyCond + ` ORDER BY s.site_id`,
		Fields: []Field{
			{"site_id", "integer", "", "測站編號"},
			{"observed_at", "timestamp", "", "觀測時間（臺灣時間）"},
			{"uvi", "number", "", "紫外線指數（0–2 低量、3–5 中量、6–7 高量、8–10 過量、11 以上危險）"},
		},
		LatestSQL: `
			SELECT o.site_id, o.observed_at, o.uvi::float8
			  FROM data.uv_obs_current o JOIN ref.uv_site s USING (site_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.site_id::text = $2)
			 ORDER BY o.site_id`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT o.site_id, o.observed_at, o.uvi::float8,
			         o.observed_at AS cursor_t, lpad(o.site_id::text, 10, '0') AS cursor_k
			    FROM data.uv_obs_history o JOIN ref.uv_site s USING (site_id)
			   WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.site_id::text = $2)
			     AND o.observed_at >= $3 AND o.observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "aqi_forecast",
		Months:      `SELECT to_char(published_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.aqi_forecast GROUP BY 1 ORDER BY 1`,
		Description: "環境部每天 10:30、16:30、22:00 發布的空氣品質預報：十個空品區未來三天的 AQI。每次發布的版本都保留。歷史資料依發布時間查詢。",
		Fields: []Field{
			{"area", "string", "", "空品區：北部、竹苗、中部、雲嘉南、高屏、宜蘭、花東、澎湖、金門、馬祖"},
			{"forecast_date", "date", "", "預報的日期"},
			{"published_at", "timestamp", "", "發布時間（臺灣時間）"},
			{"aqi", "integer", "", "預估的空氣品質指標"},
			{"major_pollutant", "string", "", "主要污染物"},
			{"minor_pollutant", "string", "", "次要污染物"},
			{"minor_aqi", "integer", "", "次要污染物的預估指標"},
			{"content", "string", "", "這次發布的預報說明文字（各空品區相同）"},
		},
		LatestSQL: `
			SELECT f.area, f.forecast_date, f.published_at, f.aqi, f.major_pollutant, f.minor_pollutant, f.minor_aqi, t.content
			  FROM data.aqi_forecast f
			  JOIN ref.aqi_area a USING (area)
			  LEFT JOIN data.aqi_forecast_text t USING (published_at)
			 WHERE $1::text IS NULL AND $2::text IS NULL
			   AND f.published_at = (SELECT max(published_at) FROM data.aqi_forecast)
			 ORDER BY a.sort_order, f.forecast_date`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT f.area, f.forecast_date, f.published_at, f.aqi, f.major_pollutant, f.minor_pollutant, f.minor_aqi, t.content,
			         f.published_at AS cursor_t, f.area || '|' || f.forecast_date::text AS cursor_k
			    FROM data.aqi_forecast f
			    LEFT JOIN data.aqi_forecast_text t USING (published_at)
			   WHERE $1::text IS NULL AND $2::text IS NULL
			     AND f.published_at >= $3 AND f.published_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "reservoir_obs",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.reservoir_obs_history GROUP BY 1 ORDER BY 1`,
		Description: "經濟部水利署的水庫即時水情（大多每小時一筆，離島水庫每天一筆）。蓄水率依本站水庫主檔的有效容量計算。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"reservoir_id", "string", "", "水庫代碼（水利署）"},
			{"name", "string", "", "水庫名稱"},
			countyField,
			{"effective_capacity", "number", "萬立方公尺", "有效容量"},
			{"is_major", "boolean", "", "是否為主要水庫"},
			{"lon", "number", "度", "經度（WGS 84，本站人工標註）"},
			{"lat", "number", "度", "緯度（WGS 84，本站人工標註）"},
		},
		SitesSQL: `
			SELECT s.reservoir_id, s.name, s.county_code, s.effective_capacity::float8, s.is_major, ST_X(s.geom), ST_Y(s.geom)
			  FROM ref.reservoir s WHERE ` + countyCond + ` ORDER BY s.reservoir_id`,
		Fields: []Field{
			{"reservoir_id", "string", "", "水庫代碼"},
			{"observed_at", "timestamp", "", "觀測時間（臺灣時間）"},
			{"water_level", "number", "公尺", "水位"},
			{"effective_storage", "number", "萬立方公尺", "有效蓄水量"},
			{"storage_pct", "number", "%", "蓄水率"},
			{"inflow", "number", "立方公尺/秒", "進水量"},
			{"total_outflow", "number", "立方公尺/秒", "總出水量"},
			{"rainfall", "number", "毫米", "集水區累積雨量"},
			{"status_type", "integer", "", "水利署的狀態代碼"},
		},
		LatestSQL: `
			SELECT o.reservoir_id, o.observed_at, o.water_level_m::float8, o.effective_storage::float8, o.storage_pct::float8,
			       o.inflow_cms::float8, o.total_outflow_cms::float8, o.rainfall_mm::float8, o.status_type
			  FROM data.reservoir_obs_current o JOIN ref.reservoir s USING (reservoir_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.reservoir_id = $2)
			 ORDER BY o.reservoir_id`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT o.reservoir_id, o.observed_at, o.water_level_m::float8, o.effective_storage::float8, o.storage_pct::float8,
			         o.inflow_cms::float8, o.total_outflow_cms::float8, o.rainfall_mm::float8, o.status_type,
			         o.observed_at AS cursor_t, o.reservoir_id::text AS cursor_k
			    FROM data.reservoir_obs_history o JOIN ref.reservoir s USING (reservoir_id)
			   WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.reservoir_id = $2)
			     AND o.observed_at >= $3 AND o.observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "river_level",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.river_level_history GROUP BY 1 ORDER BY 1`,
		Description: "經濟部水利署河川水位站每 10 分鐘的即時水位，資料未經水利署檢核。測站清單附各級警戒水位（一級最高、三級最低，部分測站只設定其中幾級）。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"station_id", "string", "", "水位站站號（水利署）"},
			{"name", "string", "", "站名"},
			{"river_name", "string", "", "所屬河川"},
			countyField,
			{"alert1_m", "number", "公尺", "一級警戒水位"},
			{"alert2_m", "number", "公尺", "二級警戒水位"},
			{"alert3_m", "number", "公尺", "三級警戒水位"},
			{"is_active", "boolean", "", "是否為現存測站"},
			{"lon", "number", "度", "經度（WGS 84，由 TWD97 座標轉換，與站址縣市不符時為空值）"},
			{"lat", "number", "度", "緯度（WGS 84）"},
		},
		SitesSQL: `
			SELECT s.station_id, s.name, s.river_name, s.county_code, s.alert1_m::float8, s.alert2_m::float8,
			       s.alert3_m::float8, s.is_active, ST_X(s.geom), ST_Y(s.geom)
			  FROM ref.river_station s WHERE ` + countyCond + ` ORDER BY s.station_id`,
		Fields: []Field{
			{"station_id", "string", "", "水位站站號"},
			{"observed_at", "timestamp", "", "觀測時間（臺灣時間）"},
			{"water_level_m", "number", "公尺", "水位"},
		},
		LatestSQL: `
			SELECT o.station_id, o.observed_at, o.water_level_m::float8
			  FROM data.river_level_current o JOIN ref.river_station s USING (station_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.station_id = $2)
			 ORDER BY o.station_id`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT o.station_id, o.observed_at, o.water_level_m::float8,
			         o.observed_at AS cursor_t, o.station_id::text AS cursor_k
			    FROM data.river_level_history o JOIN ref.river_station s USING (station_id)
			   WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.station_id = $2)
			     AND o.observed_at >= $3 AND o.observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	waterOutageDataset("water_outage", "twc",
		"台灣自來水公司的停水資訊：每件停水的影響縣市、行政區、地區、原因、影響戶數與起訖時間。本站每 30 分鐘抓一次，"+
			"公告結束或取消後保留在這裡。last_seen_at 是最後一次出現在來源清單的時間。歷史資料依停水開始時間查詢。"),
	waterOutageDataset("water_outage_taipei", "taipei",
		"臺北自來水事業處的停水公告。公告是「期間內每天幾點到幾點」：starts_at／ends_at 是第一天的開始與最後一天的結束，"+
			"daily_from／daily_to 是每天的時段。來源沒有影響戶數與案件類型。歷史資料依停水開始時間查詢。"),
	{
		ID:          "power_outage",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.power_outage_history GROUP BY 1 ORDER BY 1`,
		Description: "台灣電力公司在颱風、地震等災害期間每小時公布的停電資訊，本站保留每一次公布的內容。平時沒有資料。不含來源中的聯絡人姓名與電話。",
		Fields: []Field{
			{"observed_at", "timestamp", "", "台電的資料更新時間（臺灣時間，來源沒有時為本站抓取時間）"},
			{"seq", "integer", "", "同一次資料裡的順序"},
			countyField,
			{"district", "string", "", "行政區"},
			{"area", "string", "", "停電區域"},
			{"info", "string", "", "停電資訊"},
			{"households", "integer", "戶", "影響戶數"},
			{"expected_restore", "string", "", "預計修復時間（來源的文字）"},
		},
		LatestSQL: `
			SELECT observed_at, seq, county_code, district, area, info, households, expected_restore
			  FROM data.power_outage_current s
			 WHERE ` + countyCond + ` AND $2::text IS NULL
			 ORDER BY seq`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT observed_at, seq, county_code, district, area, info, households, expected_restore,
			         observed_at AS cursor_t, lpad(seq::text, 5, '0') AS cursor_k
			    FROM data.power_outage_history s
			   WHERE ` + countyCond + ` AND $2::text IS NULL
			     AND observed_at >= $3 AND observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
		HasCounty: true,
	},
	{
		ID:          "typhoon_track",
		Months:      `SELECT to_char(f.valid_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.typhoon_fix f GROUP BY 1 ORDER BY 1`,
		Description: "中央氣象署的熱帶氣旋路徑：kind=analysis 是實際位置，kind=forecast 是每一次發布的預報位置（issued_at 是預報起始時間）。歷史資料依位置的時間（valid_at）查詢。",
		Fields: []Field{
			{"typhoon_id", "string", "", "本站的熱帶氣旋代碼（年份-熱帶性低氣壓編號），升格為颱風後不變"},
			{"name_zh", "string", "", "中文名稱"},
			{"name_en", "string", "", "國際命名"},
			{"ty_no", "integer", "", "氣象署颱風編號（第幾號颱風）"},
			{"kind", "string", "", "analysis（實際）或 forecast（預報）"},
			{"issued_at", "timestamp", "", "實際：位置的時間。預報：預報起始時間"},
			{"forecast_hour", "integer", "小時", "預報幾小時後（實際為 0）"},
			{"valid_at", "timestamp", "", "位置的時間"},
			{"lon", "number", "度", "中心經度"},
			{"lat", "number", "度", "中心緯度"},
			{"pressure_hpa", "integer", "百帕", "中心氣壓"},
			{"max_wind_ms", "integer", "公尺/秒", "近中心最大風速"},
			{"max_gust_ms", "integer", "公尺/秒", "瞬間最大陣風"},
			{"moving_speed_kmh", "integer", "公里/小時", "移動速度"},
			{"moving_dir", "string", "", "移動方向（16 方位）"},
			{"radius7_km", "integer", "公里", "七級風暴風半徑"},
			{"radius10_km", "integer", "公里", "十級風暴風半徑"},
			{"radius70_km", "integer", "公里", "70% 機率半徑（只有預報）"},
			{"note", "string", "", "備註，例如「變性為溫帶氣旋」"},
		},
		LatestSQL: `
			SELECT ` + typhoonFixCols + `
			  FROM data.typhoon_fix f JOIN data.typhoon t USING (typhoon_id)
			 WHERE $1::text IS NULL AND $2::text IS NULL
			   AND t.last_seen_at >= now() - interval '6 hours'
			   AND (f.kind = 'analysis' OR f.issued_at = (SELECT max(issued_at) FROM data.typhoon_fix x
			                                              WHERE x.typhoon_id = f.typhoon_id AND x.kind = 'forecast'))
			 ORDER BY f.typhoon_id, f.kind, f.valid_at`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT ` + typhoonFixCols + `,
			         f.valid_at AS cursor_t, f.typhoon_id || '|' || f.kind || '|' || f.issued_at::text AS cursor_k
			    FROM data.typhoon_fix f JOIN data.typhoon t USING (typhoon_id)
			   WHERE $1::text IS NULL AND $2::text IS NULL
			     AND f.valid_at >= $3 AND f.valid_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "typhoon_warning",
		Months:      `SELECT to_char(effective_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.typhoon_warning GROUP BY 1 ORDER BY 1`,
		Description: "中央氣象署每一報颱風警報：警報種類、警戒區域與各段說明（命名與位置、強度與半徑、移速與預測、颱風動態、警戒區域及事項…）。",
		Fields: []Field{
			{"effective_at", "timestamp", "", "發布時間"},
			{"ty_no", "integer", "", "颱風編號"},
			{"warning_no", "integer", "", "第幾報"},
			{"name_zh", "string", "", "中文名稱"},
			{"name_en", "string", "", "國際命名"},
			{"alert", "string", "", "警報種類，例如海上陸上颱風警報"},
			{"expires_at", "timestamp", "", "預定下一報的時間"},
			{"areas", "string", "", "警戒區域（以「、」分隔）"},
			{"sections", "string", "", "各段說明（JSON 陣列，每項有 title 與 value）"},
			{"url", "string", "", "氣象署網頁"},
		},
		LatestSQL: `
			SELECT effective_at, ty_no, warning_no, name_zh, name_en, alert, expires_at,
			       array_to_string(areas, '、'), sections::text, url
			  FROM data.typhoon_warning
			 WHERE $1::text IS NULL AND $2::text IS NULL
			   AND effective_at = (SELECT max(effective_at) FROM data.typhoon_warning)`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT effective_at, ty_no, warning_no, name_zh, name_en, alert, expires_at,
			         array_to_string(areas, '、'), sections::text, url,
			         effective_at AS cursor_t, ty_no::text AS cursor_k
			    FROM data.typhoon_warning
			   WHERE $1::text IS NULL AND $2::text IS NULL
			     AND effective_at >= $3 AND effective_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "weather_warning",
		Months:      `SELECT to_char(starts_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.weather_warning s GROUP BY 1 ORDER BY 1`,
		Description: "中央氣象署發布到各縣市的天氣警報與特報（颱風、豪雨、強風、低溫…）。同一縣市、同一現象、同一開始時間的特報更新時延長結束時間。last_seen_at 是最後一次出現在來源的時間。歷史資料依開始時間查詢。",
		HasCounty:   true,
		Fields: []Field{
			countyField,
			{"phenomena", "string", "", "現象，例如海上陸上颱風、陸上強風、豪雨"},
			{"significance", "string", "", "警報或特報"},
			{"starts_at", "timestamp", "", "生效時間"},
			{"ends_at", "timestamp", "", "預定結束時間"},
			{"first_seen_at", "timestamp", "", "本站第一次抓到的時間"},
			{"last_seen_at", "timestamp", "", "最後一次出現在來源的時間"},
		},
		LatestSQL: `
			SELECT s.county_code, s.phenomena, s.significance, s.starts_at, s.ends_at, s.first_seen_at, s.last_seen_at
			  FROM data.weather_warning s
			 WHERE ` + countyCond + ` AND $2::text IS NULL
			   AND s.last_seen_at >= now() - interval '1 hour' AND (s.ends_at IS NULL OR s.ends_at > now())
			 ORDER BY s.county_code, s.starts_at`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT s.county_code, s.phenomena, s.significance, s.starts_at, s.ends_at, s.first_seen_at, s.last_seen_at,
			         s.starts_at AS cursor_t, s.county_code || '|' || s.phenomena || '|' || s.significance AS cursor_k
			    FROM data.weather_warning s
			   WHERE ` + countyCond + ` AND $2::text IS NULL
			     AND s.starts_at >= $3 AND s.starts_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "rain_obs",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.rain_obs_history GROUP BY 1 ORDER BY 1`,
		Description: "中央氣象署整合的雨量站（約 1,300 站，含水利署等單位）的累積雨量。最新資料每 10 分鐘更新。歷史資料是每小時整點的那一筆，past1hr_mm 就是該小時的雨量。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"station_id", "string", "", "雨量站站號（氣象署）"},
			{"name", "string", "", "站名"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"altitude_m", "number", "公尺", "海拔高度"},
			{"maintainer", "string", "", "維護單位"},
			{"lon", "number", "度", "經度（WGS 84）"},
			{"lat", "number", "度", "緯度（WGS 84）"},
		},
		SitesSQL: `
			SELECT s.station_id, s.name, s.county_code, s.town, s.altitude_m::float8, s.maintainer, ST_X(s.geom), ST_Y(s.geom)
			  FROM ref.rain_station s WHERE ` + countyCond + ` ORDER BY s.station_id`,
		Fields: []Field{
			{"station_id", "string", "", "雨量站站號"},
			{"observed_at", "timestamp", "", "觀測時間（臺灣時間）"},
			{"today_mm", "number", "毫米", "本日累積（從 0 時起）"},
			{"past10min_mm", "number", "毫米", "過去 10 分鐘"},
			{"past1hr_mm", "number", "毫米", "過去 1 小時"},
			{"past3hr_mm", "number", "毫米", "過去 3 小時"},
			{"past6hr_mm", "number", "毫米", "過去 6 小時"},
			{"past12hr_mm", "number", "毫米", "過去 12 小時"},
			{"past24hr_mm", "number", "毫米", "過去 24 小時"},
			{"past2days_mm", "number", "毫米", "過去 2 天"},
			{"past3days_mm", "number", "毫米", "過去 3 天"},
		},
		LatestSQL: `
			SELECT o.station_id, o.observed_at, ` + rainCols + `
			  FROM data.rain_obs_current o JOIN ref.rain_station s USING (station_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.station_id = $2)
			 ORDER BY o.station_id`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT o.station_id, o.observed_at, ` + rainCols + `,
			         o.observed_at AS cursor_t, o.station_id AS cursor_k
			    FROM data.rain_obs_history o JOIN ref.rain_station s USING (station_id)
			   WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.station_id = $2)
			     AND o.observed_at >= $3 AND o.observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "weather_obs",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.weather_obs_history GROUP BY 1 ORDER BY 1`,
		Description: "中央氣象署氣象站與自動站（約 870 站）的氣溫、濕度、氣壓與風。最新資料約 360 站每 10 分鐘、其他每小時更新。歷史資料是每小時整點的那一筆。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"station_id", "string", "", "測站站號（氣象署）"},
			{"name", "string", "", "站名"},
			{"kind", "string", "", "manned（有人的氣象站）或 auto（自動站）"},
			countyField,
			{"town", "string", "", "鄉鎮市區"},
			{"altitude_m", "number", "公尺", "海拔高度"},
			{"lon", "number", "度", "經度（WGS 84）"},
			{"lat", "number", "度", "緯度（WGS 84）"},
		},
		SitesSQL: `
			SELECT s.station_id, s.name, s.kind, s.county_code, s.town, s.altitude_m::float8, ST_X(s.geom), ST_Y(s.geom)
			  FROM ref.weather_station s WHERE ` + countyCond + ` ORDER BY s.station_id`,
		Fields: []Field{
			{"station_id", "string", "", "測站站號"},
			{"observed_at", "timestamp", "", "觀測時間（臺灣時間）"},
			{"weather", "string", "", "天氣現象（只有部分測站有）"},
			{"temp_c", "number", "°C", "氣溫"},
			{"humidity_pct", "number", "%", "相對濕度"},
			{"pressure_hpa", "number", "百帕", "氣壓"},
			{"wind_dir_deg", "number", "度", "風向：風從哪裡吹來，0 是北風、90 是東風"},
			{"wind_ms", "number", "公尺／秒", "10 分鐘平均風速"},
			{"gust_ms", "number", "公尺／秒", "本日最大陣風"},
			{"high_c", "number", "°C", "本日最高溫"},
			{"low_c", "number", "°C", "本日最低溫"},
		},
		LatestSQL: `
			SELECT o.station_id, o.observed_at, ` + obsCols + `
			  FROM data.weather_obs_current o JOIN ref.weather_station s USING (station_id)
			 WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.station_id = $2)
			 ORDER BY o.station_id`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT o.station_id, o.observed_at, ` + obsCols + `,
			         o.observed_at AS cursor_t, o.station_id AS cursor_k
			    FROM data.weather_obs_history o JOIN ref.weather_station s USING (station_id)
			   WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.station_id = $2)
			     AND o.observed_at >= $3 AND o.observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "county_forecast",
		Months:      `SELECT to_char(starts_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.county_forecast s GROUP BY 1 ORDER BY 1`,
		Description: "中央氣象署各縣市未來 36 小時的天氣預報，每 12 小時一個時段。同一個時段之後發布的預報會覆寫前面的，所以歷史資料是每個時段最後一次的預報。歷史資料依時段的開始時間查詢。",
		HasCounty:   true,
		Fields: []Field{
			countyField,
			{"starts_at", "timestamp", "", "時段開始"},
			{"ends_at", "timestamp", "", "時段結束"},
			{"weather", "string", "", "天氣現象，例如多雲時晴"},
			{"weather_code", "integer", "", "氣象署的天氣現象代碼"},
			{"rain_chance_pct", "integer", "%", "降雨機率"},
			{"min_temp_c", "integer", "°C", "最低溫"},
			{"max_temp_c", "integer", "°C", "最高溫"},
			{"comfort", "string", "", "舒適度"},
		},
		LatestSQL: `
			SELECT s.county_code, s.starts_at, s.ends_at, s.weather, s.weather_code, s.rain_chance_pct,
			       s.min_temp_c, s.max_temp_c, s.comfort
			  FROM data.county_forecast s
			 WHERE ` + countyCond + ` AND $2::text IS NULL AND s.ends_at > now()
			 ORDER BY s.county_code, s.starts_at`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT s.county_code, s.starts_at, s.ends_at, s.weather, s.weather_code, s.rain_chance_pct,
			         s.min_temp_c, s.max_temp_c, s.comfort,
			         s.starts_at AS cursor_t, s.county_code::text AS cursor_k
			    FROM data.county_forecast s
			   WHERE ` + countyCond + ` AND $2::text IS NULL
			     AND s.starts_at >= $3 AND s.starts_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	earthquakeDataset("earthquake", "significant", "中央氣象署的顯著有感地震報告（有年度編號）。"),
	earthquakeDataset("earthquake_local", "local", "中央氣象署的小區域有感地震報告（影響範圍較小，沒有年度編號）。"),
	{
		ID:          "power_gen",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.power_gen_history GROUP BY 1 ORDER BY 1`,
		Description: "台灣電力公司各機組的即時發電量（每 10 分鐘），含民營電廠。",
		HasCounty:   true, HasSite: true,
		SiteFields: []Field{
			{"unit_id", "integer", "", "機組編號（本站編號）"},
			{"unit_name", "string", "", "機組名稱（台電）"},
			{"plant_name", "string", "", "電廠名稱"},
			{"fuel_type", "string", "", "燃料：nuclear、coal、cogen、gas、oil、hydro、wind、solar、other_renewable、storage、storage_load"},
			{"is_private", "boolean", "", "是否為民營電廠"},
			countyField,
		},
		SitesSQL: `
			SELECT s.unit_id, s.unit_name, s.plant_name, s.fuel_type, s.is_private, s.county_code
			  FROM ref.power_unit s WHERE ` + countyCond + ` ORDER BY s.unit_id`,
		Fields: []Field{
			{"unit_id", "integer", "", "機組編號"},
			{"observed_at", "timestamp", "", "資料時間（臺灣時間）"},
			{"capacity_mw", "number", "MW", "裝置容量"},
			{"net_gen_mw", "number", "MW", "淨發電量"},
			{"remark", "string", "", "台電的備註（例如歲修、故障）"},
		},
		LatestSQL: `
			SELECT o.unit_id, o.observed_at, o.capacity_mw::float8, o.net_gen_mw::float8, o.remark
			  FROM data.power_gen_history o JOIN ref.power_unit s USING (unit_id)
			 WHERE o.observed_at = (SELECT max(observed_at) FROM data.power_gen_history)
			   AND ` + countyCond + ` AND ($2::text IS NULL OR o.unit_id::text = $2)
			 ORDER BY o.unit_id`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT o.unit_id, o.observed_at, o.capacity_mw::float8, o.net_gen_mw::float8, o.remark,
			         o.observed_at AS cursor_t, lpad(o.unit_id::text, 10, '0') AS cursor_k
			    FROM data.power_gen_history o JOIN ref.power_unit s USING (unit_id)
			   WHERE ` + countyCond + ` AND ($2::text IS NULL OR o.unit_id::text = $2)
			     AND o.observed_at >= $3 AND o.observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "power_region",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.power_region_history GROUP BY 1 ORDER BY 1`,
		Description: "台灣電力公司北、中、南、東四區的即時發電量與用電量（每 10 分鐘）。發電量依電廠所在地分區，用電量再依區域之間的電力潮流調整。",
		Fields: []Field{
			{"region", "string", "", "區域：北部、中部、南部、東部"},
			{"observed_at", "timestamp", "", "資料時間（臺灣時間）"},
			{"gen_mw", "number", "MW", "發電量"},
			{"load_mw", "number", "MW", "用電量"},
		},
		LatestSQL: `
			SELECT region, observed_at, gen_mw::float8, load_mw::float8
			  FROM data.power_region_history
			 WHERE $1::text IS NULL AND $2::text IS NULL
			   AND observed_at = (SELECT max(observed_at) FROM data.power_region_history)
			 ORDER BY array_position(ARRAY['北部', '中部', '南部', '東部'], region)`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT region, observed_at, gen_mw::float8, load_mw::float8,
			         observed_at AS cursor_t, region AS cursor_k
			    FROM data.power_region_history
			   WHERE $1::text IS NULL AND $2::text IS NULL
			     AND observed_at >= $3 AND observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "power_load",
		Months:      `SELECT to_char(observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.power_load_history GROUP BY 1 ORDER BY 1`,
		Description: "台灣電力公司的全國即時用電量與使用率，以及當天預估的最大供電能力與尖峰備轉（每 10 分鐘）。",
		Fields: []Field{
			{"observed_at", "timestamp", "", "台電的發布時間（臺灣時間）"},
			{"load_mw", "number", "MW", "目前用電量"},
			{"util_pct", "number", "%", "使用率"},
			{"fore_supply_mw", "number", "MW", "今日預估最大供電能力"},
			{"fore_peak_load_mw", "number", "MW", "今日預估尖峰負載"},
			{"fore_reserve_mw", "number", "MW", "今日預估尖峰備轉容量"},
			{"fore_reserve_pct", "number", "%", "今日預估尖峰備轉容量率"},
			{"fore_indicator", "string", "", "今日供電燈號：G 綠、Y 黃、O 橘、R 紅、B 黑"},
			{"fore_peak_hours", "string", "", "今日預估尖峰時段"},
		},
		LatestSQL: `
			SELECT observed_at, load_mw::float8, util_pct::float8, fore_supply_mw::float8, fore_peak_load_mw::float8,
			       fore_reserve_mw::float8, fore_reserve_pct::float8, fore_indicator, fore_peak_hours
			  FROM data.power_load_history
			 WHERE $1::text IS NULL AND $2::text IS NULL
			 ORDER BY observed_at DESC LIMIT 1`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT observed_at, load_mw::float8, util_pct::float8, fore_supply_mw::float8, fore_peak_load_mw::float8,
			         fore_reserve_mw::float8, fore_reserve_pct::float8, fore_indicator, fore_peak_hours,
			         observed_at AS cursor_t, '' AS cursor_k
			    FROM data.power_load_history
			   WHERE $1::text IS NULL AND $2::text IS NULL
			     AND observed_at >= $3 AND observed_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "power_peak_daily",
		Months:      `SELECT to_char(date, 'YYYY-MM'), count(*) FROM data.power_peak_daily GROUP BY 1 ORDER BY 1`,
		Description: "台灣電力公司每天公布的前一天實際尖峰：最大供電能力、尖峰負載與備轉容量率。比按月公布的過去電力供需早一個多月，數字可能與之後的正式資料略有差異。",
		Fields: []Field{
			{"date", "date", "", "日期"},
			{"supply_mw", "number", "MW", "最大供電能力"},
			{"peak_load_mw", "number", "MW", "尖峰負載"},
			{"reserve_mw", "number", "MW", "尖峰備轉容量"},
			{"reserve_pct", "number", "%", "尖峰備轉容量率"},
			{"indicator", "string", "", "供電燈號：G 綠、Y 黃、O 橘、R 紅、B 黑"},
		},
		LatestSQL: `
			SELECT date, supply_mw::float8, peak_load_mw::float8, reserve_mw::float8, reserve_pct::float8, indicator
			  FROM data.power_peak_daily
			 WHERE $1::text IS NULL AND $2::text IS NULL
			 ORDER BY date DESC LIMIT 1`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT date, supply_mw::float8, peak_load_mw::float8, reserve_mw::float8, reserve_pct::float8, indicator,
			         (date::timestamp AT TIME ZONE 'Asia/Taipei') AS cursor_t, '' AS cursor_k
			    FROM data.power_peak_daily
			   WHERE $1::text IS NULL AND $2::text IS NULL
			     AND date >= ($3::timestamptz AT TIME ZONE 'Asia/Taipei')::date
			     AND date < ($4::timestamptz AT TIME ZONE 'Asia/Taipei')::date) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "power_supply_daily",
		Months:      `SELECT to_char(date, 'YYYY-MM'), count(*) FROM data.power_supply_daily GROUP BY 1 ORDER BY 1`,
		Description: "台灣電力公司過去每日的電力供需：淨尖峰供電能力、尖峰負載、備轉容量與用電量（台電按月公布）。",
		Fields: []Field{
			{"date", "date", "", "日期"},
			{"net_peak_capacity_mw", "number", "MW", "淨尖峰供電能力"},
			{"peak_load_mw", "number", "MW", "尖峰負載"},
			{"reserve_mw", "number", "MW", "備轉容量"},
			{"reserve_pct", "number", "%", "備轉容量率"},
			{"industrial_gwh", "number", "GWh", "工業用電"},
			{"residential_gwh", "number", "GWh", "民生用電"},
		},
		LatestSQL: `
			SELECT date, net_peak_capacity_mw::float8, peak_load_mw::float8, reserve_mw::float8, reserve_pct::float8,
			       industrial_gwh::float8, residential_gwh::float8
			  FROM data.power_supply_daily
			 WHERE $1::text IS NULL AND $2::text IS NULL
			 ORDER BY date DESC LIMIT 1`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT date, net_peak_capacity_mw::float8, peak_load_mw::float8, reserve_mw::float8, reserve_pct::float8,
			         industrial_gwh::float8, residential_gwh::float8,
			         (date::timestamp AT TIME ZONE 'Asia/Taipei') AS cursor_t, '' AS cursor_k
			    FROM data.power_supply_daily
			   WHERE $1::text IS NULL AND $2::text IS NULL
			     AND date >= ($3::timestamptz AT TIME ZONE 'Asia/Taipei')::date
			     AND date < ($4::timestamptz AT TIME ZONE 'Asia/Taipei')::date) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
	{
		ID:          "power_supply_forecast",
		Months:      `SELECT to_char(forecast_date, 'YYYY-MM'), count(*) FROM data.power_supply_forecast GROUP BY 1 ORDER BY 1`,
		Description: "台灣電力公司未來兩個月的每日電力供需預測。每次公布的版本都保留，published_on 是本站第一次抓到該版本的日期。",
		Fields: []Field{
			{"forecast_date", "date", "", "預測的日期"},
			{"published_on", "date", "", "這個版本的公布日期（本站第一次抓到）"},
			{"net_peak_capacity_mw", "number", "MW", "預估淨尖峰供電能力"},
			{"peak_load_mw", "number", "MW", "預估尖峰負載"},
			{"reserve_mw", "number", "MW", "預估備轉容量"},
			{"reserve_pct", "number", "%", "預估備轉容量率"},
		},
		LatestSQL: `
			SELECT forecast_date, published_on, net_peak_capacity_mw::float8, peak_load_mw::float8,
			       reserve_mw::float8, reserve_pct::float8
			  FROM data.power_supply_forecast
			 WHERE $1::text IS NULL AND $2::text IS NULL
			   AND published_on = (SELECT max(published_on) FROM data.power_supply_forecast)
			 ORDER BY forecast_date`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT forecast_date, published_on, net_peak_capacity_mw::float8, peak_load_mw::float8,
			         reserve_mw::float8, reserve_pct::float8,
			         (forecast_date::timestamp AT TIME ZONE 'Asia/Taipei') AS cursor_t, published_on::text AS cursor_k
			    FROM data.power_supply_forecast
			   WHERE $1::text IS NULL AND $2::text IS NULL
			     AND forecast_date >= ($3::timestamptz AT TIME ZONE 'Asia/Taipei')::date
			     AND forecast_date < ($4::timestamptz AT TIME ZONE 'Asia/Taipei')::date) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	},
}

func Find(id string) *Dataset {
	for i := range Datasets {
		if Datasets[i].ID == id {
			return &Datasets[i]
		}
	}
	return nil
}

func waterOutageDataset(id, source, description string) Dataset {
	src := `s.source = '` + source + `'`
	cols := `s.case_id, s.county_code, s.district, s.area, s.reason, s.kind, s.households, s.starts_at, s.ends_at,
	         to_char(s.daily_from, 'HH24:MI'), to_char(s.daily_to, 'HH24:MI'), s.low_pressure_area, s.low_pressure_reason,
	         s.url, s.published_on, s.first_seen_at, s.last_seen_at`
	return Dataset{
		ID:          id,
		Months:      `SELECT to_char(starts_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.water_outage s WHERE ` + src + ` GROUP BY 1 ORDER BY 1`,
		Description: description,
		HasCounty:   true,
		Fields: []Field{
			{"case_id", "string", "", "來源的案件編號"},
			countyField,
			{"district", "string", "", "影響行政區（可能有多個，以逗號分隔）"},
			{"area", "string", "", "停水地區"},
			{"reason", "string", "", "停水原因"},
			{"kind", "string", "", "計畫性、非計畫性（臺北自來水沒有）"},
			{"households", "integer", "戶", "影響戶數（臺北自來水沒有）"},
			{"starts_at", "timestamp", "", "停水開始時間（臺灣時間）"},
			{"ends_at", "timestamp", "", "預計恢復供水時間（臺灣時間）"},
			{"daily_from", "string", "", "每天的停水開始時刻（臺北自來水）"},
			{"daily_to", "string", "", "每天的停水結束時刻（臺北自來水）"},
			{"low_pressure_area", "string", "", "降壓地區"},
			{"low_pressure_reason", "string", "", "降壓原因"},
			{"url", "string", "", "來源的公告網址"},
			{"published_on", "date", "", "公告日期"},
			{"first_seen_at", "timestamp", "", "本站第一次抓到這件公告的時間"},
			{"last_seen_at", "timestamp", "", "最後一次出現在來源清單的時間"},
		},
		LatestSQL: `
			SELECT ` + cols + `
			  FROM data.water_outage s
			 WHERE ` + src + ` AND ` + countyCond + ` AND $2::text IS NULL
			   AND s.last_seen_at >= now() - interval '2 hours'
			 ORDER BY s.starts_at, s.case_id`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT ` + cols + `,
			         s.starts_at AS cursor_t, s.case_id AS cursor_k
			    FROM data.water_outage s
			   WHERE ` + src + ` AND ` + countyCond + ` AND $2::text IS NULL
			     AND s.starts_at >= $3 AND s.starts_at < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	}
}

func earthquakeDataset(id, kind, description string) Dataset {
	cond := `e.kind = '` + kind + `' AND $1::text IS NULL AND $2::text IS NULL`
	cols := `e.report_id, e.earthquake_no, e.origin_time, e.issued_at, e.magnitude::float8, e.depth_km::float8,
	         e.location, ST_X(e.geom), ST_Y(e.geom), e.max_intensity, e.report_color, e.content, e.report_url, e.image_url,
	         (SELECT string_agg(c.name || ' ' || i.intensity, '、' ORDER BY i.intensity_rank DESC, c.sort_order)
	            FROM data.earthquake_intensity i JOIN ref.county c ON c.code = i.county_code
	           WHERE i.report_id = e.report_id)`
	return Dataset{
		ID:     id,
		Months: `SELECT to_char(origin_time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM'), count(*) FROM data.earthquake e WHERE e.kind = '` + kind + `' GROUP BY 1 ORDER BY 1`,
		Description: description + "氣象署修正報告時本站會更新。county_intensities 列出各縣市的最大震度。" +
			"歷史資料依發震時間查詢。",
		Fields: []Field{
			{"report_id", "string", "", "報告編號（氣象署報告網址的最後一段）"},
			{"earthquake_no", "integer", "", "地震編號（小區域報告沒有實際編號）"},
			{"origin_time", "timestamp", "", "發震時間（臺灣時間）"},
			{"issued_at", "timestamp", "", "報告發布時間（臺灣時間）"},
			{"magnitude", "number", "", "芮氏規模"},
			{"depth_km", "number", "公里", "震源深度"},
			{"location", "string", "", "震央位置"},
			{"lon", "number", "度", "震央經度（WGS 84）"},
			{"lat", "number", "度", "震央緯度（WGS 84）"},
			{"max_intensity", "string", "", "各縣市中最大的震度（1級–4級、5弱、5強、6弱、6強、7級）"},
			{"report_color", "string", "", "報告顏色：綠色、黃色、橘色、紅色"},
			{"content", "string", "", "報告內容"},
			{"report_url", "string", "", "氣象署的報告網址"},
			{"image_url", "string", "", "氣象署的地震報告圖網址"},
			{"county_intensities", "string", "", "各縣市的最大震度，例如「花蓮縣 3級、南投縣 2級」"},
		},
		LatestSQL: `
			SELECT ` + cols + `
			  FROM data.earthquake e
			 WHERE ` + cond + ` AND e.origin_time >= now() - interval '30 days'
			 ORDER BY e.origin_time DESC`,
		HistorySQL: `
			SELECT * FROM (
			  SELECT ` + cols + `,
			         e.origin_time AS cursor_t, e.report_id AS cursor_k
			    FROM data.earthquake e
			   WHERE ` + cond + `
			     AND e.origin_time >= $3 AND e.origin_time < $4) q
			 WHERE ` + pageCond + `
			 ORDER BY cursor_t, cursor_k LIMIT $7`,
	}
}
