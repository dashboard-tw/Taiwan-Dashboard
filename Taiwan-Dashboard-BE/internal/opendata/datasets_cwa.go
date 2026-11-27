package opendata

// 氣象署進階會員後加入的資料集：鄉鎮預報、潮汐、日出日沒、海嘯
var cwaDatasets = []Dataset{
	{
		ID: "town_forecast",
		Description: "中央氣象署的鄉鎮未來 3 天天氣預報，368 個鄉鎮市區，每 3 小時一個時段。latest 是還沒結束的時段（分頁）。" +
			"同一個時段之後發布的預報會覆寫前面的，結束兩天後刪除，所以沒有歷史資料。鄉鎮清單在 sites。本站每小時更新一次。",
		HasCounty: true, HasSite: true, LatestPaged: true,
		SiteFields: townSiteFields,
		SitesSQL:   townSitesSQL,
		LatestFields: []Field{
			{"geocode", "string", "", "鄉鎮代碼"},
			{"starts_at", "timestamp", "", "時段開始"},
			{"ends_at", "timestamp", "", "時段結束"},
			{"weather", "string", "", "天氣現象，例如多雲時晴"},
			{"weather_code", "integer", "", "氣象署的天氣現象代碼"},
			{"rain_chance_pct", "integer", "%", "3 小時降雨機率"},
			{"temp_c", "integer", "°C", "時段開始的溫度"},
			{"apparent_c", "integer", "°C", "時段開始的體感溫度"},
			{"humidity_pct", "integer", "%", "相對濕度"},
			{"wind_ms", "integer", "公尺/秒", "風速"},
			{"beaufort", "integer", "級", "蒲福風級"},
			{"wind_dir", "string", "", "風向，例如偏北風"},
			{"comfort", "string", "", "舒適度，例如稍有寒意"},
		},
		LatestSQL: listLatest(`f.geocode, f.starts_at, f.ends_at, f.weather, f.weather_code, f.rain_chance_pct, f.temp_c,
			f.apparent_c, f.humidity_pct, f.wind_ms, f.beaufort, f.wind_dir, f.comfort`,
			`data.town_forecast f JOIN ref.forecast_town s USING (geocode)`,
			countyCond+` AND ($2::text IS NULL OR f.geocode = $2) AND f.ends_at > now()`, townPeriodKey),
	},
	{
		ID: "town_forecast_week",
		Description: "中央氣象署的鄉鎮未來 1 週天氣預報，每 12 小時一個時段，含紫外線指數（只有白天時段）。latest 是還沒結束的時段（分頁）。" +
			"結束兩天後刪除，所以沒有歷史資料。鄉鎮清單在 sites。本站每 3 小時更新一次。",
		HasCounty: true, HasSite: true, LatestPaged: true,
		SiteFields: townSiteFields,
		SitesSQL:   townSitesSQL,
		LatestFields: []Field{
			{"geocode", "string", "", "鄉鎮代碼"},
			{"starts_at", "timestamp", "", "時段開始"},
			{"ends_at", "timestamp", "", "時段結束"},
			{"weather", "string", "", "天氣現象"},
			{"weather_code", "integer", "", "氣象署的天氣現象代碼"},
			{"rain_chance_pct", "integer", "%", "12 小時降雨機率，部分時段沒有"},
			{"max_temp_c", "integer", "°C", "最高溫"},
			{"min_temp_c", "integer", "°C", "最低溫"},
			{"max_apparent_c", "integer", "°C", "最高體感溫度"},
			{"min_apparent_c", "integer", "°C", "最低體感溫度"},
			{"humidity_pct", "integer", "%", "平均相對濕度"},
			{"uvi", "integer", "", "紫外線指數，只有白天時段"},
			{"uv_level", "string", "", "紫外線等級，例如中量級"},
			{"wind_ms", "integer", "公尺/秒", "風速"},
			{"beaufort", "integer", "級", "蒲福風級"},
			{"wind_dir", "string", "", "風向"},
			{"description", "string", "", "天氣預報綜合描述"},
		},
		LatestSQL: listLatest(`f.geocode, f.starts_at, f.ends_at, f.weather, f.weather_code, f.rain_chance_pct, f.max_temp_c,
			f.min_temp_c, f.max_apparent_c, f.min_apparent_c, f.humidity_pct, f.uvi, f.uv_level, f.wind_ms, f.beaufort,
			f.wind_dir, f.description`,
			`data.town_forecast_week f JOIN ref.forecast_town s USING (geocode)`,
			countyCond+` AND ($2::text IS NULL OR f.geocode = $2) AND f.ends_at > now()`, townPeriodKey),
	},
	{
		ID: "tide_forecast",
		Description: "中央氣象署的潮汐預報，涵蓋沿海鄉鎮、漁港、海水浴場和潛點，未來約一個月每次滿潮與乾潮的時間和潮高。" +
			"latest 是還沒發生的潮汐（分頁）。兩天以前的資料會刪除，所以沒有歷史資料。地點清單在 sites。本站每天更新一次。",
		HasCounty: true, HasSite: true, LatestPaged: true,
		SiteFields: []Field{
			{"station_id", "string", "", "地點代碼"},
			{"name", "string", "", "地點名稱"},
			countyField,
			lonField, latField,
		},
		SitesSQL: `
			SELECT s.station_id, s.name, s.county_code, ` + lonLat + `
			  FROM ref.tide_station s WHERE ` + countyCond + ` ORDER BY s.station_id`,
		LatestFields: []Field{
			{"station_id", "string", "", "地點代碼"},
			{"at", "timestamp", "", "滿潮或乾潮的時間"},
			{"tide", "string", "", "滿潮或乾潮"},
			{"height_cm", "integer", "公分", "潮高（相對當地平均海平面）"},
			{"tide_range", "string", "", "當天潮差：大、中、小"},
			{"lunar_date", "string", "", "農曆日期"},
		},
		LatestSQL: listLatest(`f.station_id, f.at, f.tide, f.height_cm, f.tide_range, f.lunar_date`,
			`data.tide_forecast f JOIN ref.tide_station s USING (station_id)`,
			countyCond+` AND ($2::text IS NULL OR f.station_id = $2) AND f.at > now()`,
			`f.station_id || to_char(f.at AT TIME ZONE 'Asia/Taipei', 'YYYYMMDDHH24MI')`),
	},
	{
		ID:          "sun_moon",
		Description: "中央氣象署各縣市每天的日出、日沒、月出、月沒時刻與方位（臺灣時間）。latest 是今天起的 30 天。歷史資料依日期查詢。",
		HasCounty:   true,
		Fields: []Field{
			countyField,
			{"date", "date", "", "日期"},
			{"civil_dawn", "string", "", "民用曙光始，例如 06:16"},
			{"sunrise", "string", "", "日出"},
			{"sunrise_az", "integer", "度", "日出方位角"},
			{"sun_transit", "string", "", "太陽過中天"},
			{"sun_transit_alt", "string", "", "過中天仰角，例如 44S"},
			{"sunset", "string", "", "日沒"},
			{"sunset_az", "integer", "度", "日沒方位角"},
			{"civil_dusk", "string", "", "民用暮光終"},
			{"moonrise", "string", "", "月出，當天沒有月出時為空值"},
			{"moonrise_az", "integer", "度", "月出方位角"},
			{"moon_transit", "string", "", "月球過中天"},
			{"moon_transit_alt", "string", "", "月球過中天仰角"},
			{"moonset", "string", "", "月沒，當天沒有月沒時為空值"},
			{"moonset_az", "integer", "度", "月沒方位角"},
		},
		LatestSQL: `
			SELECT ` + sunMoonCols + ` FROM data.sun_moon s
			 WHERE ` + countyOnly + ` AND s.date >= (now() AT TIME ZONE 'Asia/Taipei')::date
			 ORDER BY s.date, s.county_code`,
		HistorySQL: history(sunMoonCols, `data.sun_moon s`, countyOnly, dayStart(`s.date`), `s.county_code::text`),
		Months:     `SELECT to_char(date, 'YYYY-MM'), count(*) FROM data.sun_moon GROUP BY 1 ORDER BY 1`,
	},
	{
		ID:          "tsunami",
		Description: "中央氣象署的海嘯消息與海嘯警報，每一報一列。多數是國外大地震的通報。latest 是近一年的資料。歷史資料依發布時間查詢。",
		Fields: []Field{
			{"tsunami_no", "integer", "", "海嘯編號"},
			{"report_seq", "integer", "", "第幾報"},
			{"issued_at", "timestamp", "", "發布時間"},
			{"valid_until", "timestamp", "", "有效時間"},
			{"report_type", "string", "", "海嘯消息或海嘯警報"},
			{"report_color", "string", "", "綠色、黃色、橘色、紅色"},
			{"content", "string", "", "內容"},
			{"report_url", "string", "", "氣象署的報告網址"},
			{"origin_time", "timestamp", "", "地震發生時間"},
			{"source", "string", "", "通報單位"},
			{"location", "string", "", "震央位置"},
			lonField, latField,
			{"magnitude", "number", "", "地震規模"},
			{"depth_km", "number", "公里", "震源深度"},
		},
		LatestSQL: `
			SELECT ` + tsunamiCols + ` FROM data.tsunami s
			 WHERE ` + noFilter + ` AND s.issued_at >= now() - interval '1 year'
			 ORDER BY s.issued_at DESC`,
		HistorySQL: history(tsunamiCols, `data.tsunami s`, noFilter, `s.issued_at`,
			`lpad(s.tsunami_no::text, 10, '0') || lpad(s.report_seq::text, 3, '0')`),
		Months: months(`issued_at`, `data.tsunami`),
	},
	{
		ID: "weather_advisory",
		Description: "中央氣象署的低溫特報、高溫資訊與濃霧特報（這三種不在 weather_warning 裡）。每次發布一列，影響地區合併成一個欄位。" +
			"latest 是近 30 天發布的。歷史資料依生效時間查詢。",
		Fields: []Field{
			{"event", "string", "", "低溫、高溫或濃霧"},
			{"effective", "timestamp", "", "生效時間"},
			{"onset", "timestamp", "", "開始時間"},
			{"expires", "timestamp", "", "結束時間"},
			{"headline", "string", "", "標題，例如低溫特報"},
			{"level", "string", "", "燈號，例如低溫黃色燈號"},
			{"color", "string", "", "燈號顏色：黃色、橙色、紅色"},
			{"description", "string", "", "說明"},
			{"instruction", "string", "", "建議的防範措施"},
			{"web", "string", "", "氣象署網頁"},
			{"areas", "string", "", "影響地區，以頓號分隔，例如新北市平溪區、彰化縣"},
		},
		LatestSQL: `
			SELECT ` + advisoryCols + ` FROM data.weather_advisory s
			 WHERE ` + noFilter + ` AND s.effective >= now() - interval '30 days'
			 ORDER BY s.effective DESC`,
		HistorySQL: history(advisoryCols, `data.weather_advisory s`, noFilter, `s.effective`, `s.event`),
		Months:     months(`effective`, `data.weather_advisory`),
	},
	{
		ID: "health_weather",
		Description: "中央氣象署的健康氣象，各鄉鎮未來 5 天的熱傷害指數（逐 3 小時）、冷傷害指數與溫差提醒（逐日）。" +
			"latest 是還沒結束的時段（分頁），可以用 site 指定鄉鎮。結束兩天後刪除，所以沒有歷史資料。鄉鎮清單在 sites。本站每 3 小時更新一次。",
		HasCounty: true, HasSite: true, LatestPaged: true,
		SiteFields: townSiteFields,
		SitesSQL:   townSitesSQL,
		LatestFields: []Field{
			{"geocode", "string", "", "鄉鎮代碼"},
			{"kind", "string", "", "heat（熱傷害）、cold（冷傷害）、temp_diff（溫差提醒）"},
			{"starts_at", "timestamp", "", "時段開始"},
			{"ends_at", "timestamp", "", "時段結束"},
			{"index_value", "integer", "", "熱傷害指數或冷傷害指數，溫差提醒沒有"},
			{"max_temp_c", "integer", "°C", "溫差提醒的最高溫"},
			{"min_temp_c", "integer", "°C", "溫差提醒的最低溫"},
			{"warning", "string", "", "提醒等級：注意、警戒、危險、高危險，沒有提醒時為空值"},
		},
		LatestSQL: listLatest(`h.geocode, h.kind, h.starts_at, h.ends_at, h.index_value, h.max_temp_c, h.min_temp_c, h.warning`,
			`data.town_health h JOIN ref.forecast_town s USING (geocode)`,
			countyCond+` AND ($2::text IS NULL OR h.geocode = $2) AND h.ends_at > now()`,
			`h.geocode || h.kind || to_char(h.starts_at AT TIME ZONE 'Asia/Taipei', 'YYYYMMDDHH24')`),
	},
	{
		ID: "climate",
		Description: "中央氣象署有人氣象站今年的每日雨量（約晚兩天公布）。微量雨記為 0。latest 是最近 30 天。歷史資料依日期查詢。" +
			"各站各月的平年值在 sites。",
		HasSite: true,
		SiteFields: []Field{
			{"station_id", "string", "", "測站代碼"},
			{"station_name", "string", "", "測站名稱"},
			{"month", "integer", "", "月份"},
			{"temp_mean_c", "number", "°C", "平均氣溫的平年值"},
			{"temp_max_c", "number", "°C", "平均最高溫"},
			{"temp_min_c", "number", "°C", "平均最低溫"},
			{"rain_mm", "number", "毫米", "月雨量的平年值"},
			{"rain_days", "number", "天", "雨量 0.1 毫米以上的日數"},
			{"start_year", "integer", "", "統計起始年"},
			{"end_year", "integer", "", "統計結束年"},
		},
		SitesSQL: `
			SELECT s.station_id, s.station_name, s.month, s.temp_mean_c::float8, s.temp_max_c::float8,
			       s.temp_min_c::float8, s.rain_mm::float8, s.rain_days::float8, s.start_year, s.end_year
			  FROM data.climate_normal s WHERE $1::text IS NULL ORDER BY s.station_id, s.month`,
		Fields: []Field{
			{"station_id", "string", "", "測站代碼"},
			{"station_name", "string", "", "測站名稱"},
			{"date", "date", "", "日期"},
			{"rain_mm", "number", "毫米", "日雨量，缺測時為空值"},
		},
		LatestSQL: `
			SELECT s.station_id, s.station_name, s.date, s.rain_mm::float8 FROM data.rain_daily s
			 WHERE $1::text IS NULL AND ($2::text IS NULL OR s.station_id = $2)
			   AND s.date >= (SELECT max(date) FROM data.rain_daily) - 30
			 ORDER BY s.date, s.station_id`,
		HistorySQL: history(`s.station_id, s.station_name, s.date, s.rain_mm::float8`, `data.rain_daily s`,
			`$1::text IS NULL AND ($2::text IS NULL OR s.station_id = $2)`, dayStart(`s.date`), `s.station_id`),
		Months: `SELECT to_char(date, 'YYYY-MM'), count(*) FROM data.rain_daily GROUP BY 1 ORDER BY 1`,
	},
}

var townSiteFields = []Field{
	{"geocode", "string", "", "鄉鎮代碼（8 碼）"},
	countyField,
	{"name", "string", "", "鄉鎮市區名稱"},
	lonField, latField,
}

const (
	townSitesSQL = `
			SELECT s.geocode, s.county_code, s.name, ` + lonLat + `
			  FROM ref.forecast_town s WHERE ` + countyCond + ` ORDER BY s.geocode`
	townPeriodKey = `f.geocode || to_char(f.starts_at AT TIME ZONE 'Asia/Taipei', 'YYYYMMDDHH24')`
	sunMoonCols   = `s.county_code, s.date, to_char(s.civil_dawn, 'HH24:MI'), to_char(s.sunrise, 'HH24:MI'), s.sunrise_az,
	         to_char(s.sun_transit, 'HH24:MI'), s.sun_transit_alt, to_char(s.sunset, 'HH24:MI'), s.sunset_az,
	         to_char(s.civil_dusk, 'HH24:MI'), to_char(s.moonrise, 'HH24:MI'), s.moonrise_az,
	         to_char(s.moon_transit, 'HH24:MI'), s.moon_transit_alt, to_char(s.moonset, 'HH24:MI'), s.moonset_az`
	advisoryCols = `s.event, s.effective, s.onset, s.expires, s.headline, s.level, s.color, s.description, s.instruction,
	         s.web, (SELECT string_agg(c.name || a.town, '、' ORDER BY c.sort_order, a.town)
	                   FROM data.weather_advisory_area a JOIN ref.county c ON c.code = a.county_code
	                  WHERE a.event = s.event AND a.effective = s.effective)`
	tsunamiCols = `s.tsunami_no, s.report_seq, s.issued_at, s.valid_until, s.report_type, s.report_color, s.content,
	         s.report_url, s.origin_time, s.source, s.location, ST_X(s.geom), ST_Y(s.geom), s.magnitude::float8,
	         s.depth_km::float8`
)

func init() {
	for _, d := range cwaDatasets {
		topicOf[d.ID] = "weather"
	}
}
