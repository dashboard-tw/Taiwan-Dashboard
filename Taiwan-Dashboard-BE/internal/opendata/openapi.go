package opendata

import (
	"fmt"
	"strings"
)

type SpecOptions struct {
	Version                                          string
	PerDay                                           int
	PerMinute                                        int
	MBPerDay                                         int
	PartnerPerDay, PartnerPerMinute, PartnerMBPerDay int
	TokenHours                                       int
	Names                                            map[string]string
	Downloads                                        bool
}

type obj = map[string]any

func fieldSchema(f Field) obj {
	s := obj{}
	switch f.Type {
	case "integer", "number", "boolean", "string":
		s["type"] = []string{f.Type, "null"}
	case "timestamp":
		s["type"] = []string{"string", "null"}
		s["format"] = "date-time"
	case "date":
		s["type"] = []string{"string", "null"}
		s["format"] = "date"
	default:
		s["type"] = []string{"string", "null"}
	}
	desc := f.Description
	if f.Unit != "" {
		desc += "（單位：" + f.Unit + "）"
	}
	s["description"] = desc
	return s
}

func rowSchema(fields []Field) obj {
	props := obj{}
	names := make([]string, len(fields))
	for i, f := range fields {
		props[f.Name] = fieldSchema(f)
		names[i] = f.Name
	}
	return obj{"type": "object", "properties": props, "required": names}
}

func ref(name string) obj { return obj{"$ref": "#/components/schemas/" + name} }

func envelope(data obj, meta string) obj {
	return obj{
		"type":       "object",
		"properties": obj{"data": data, "meta": ref(meta)},
		"required":   []string{"data", "meta"},
	}
}

func param(name, in, desc string, schema obj, required bool) obj {
	return obj{"name": name, "in": in, "description": desc, "schema": schema, "required": required}
}

var (
	countyParam = param("county", "query", "縣市代碼（內政部，五位數字），例如臺北市 63000。不填為全國",
		obj{"type": "string", "pattern": `^\d{5}$`, "examples": []string{"63000"}}, false)
	formatParam = param("format", "query", "回應格式。csv 會直接下載含 BOM 的 CSV 檔（Excel 可直接開啟）",
		obj{"type": "string", "enum": []string{"json", "csv"}, "default": "json"}, false)
)

func errorResponses(codes ...int) obj {
	text := map[int]string{
		400: "參數錯誤（例如縣市代碼格式不對、時間範圍超過 31 天）",
		401: "invalid_token：沒有帶 Access Token，或 token 無效、已過期",
		404: "找不到資料集，或這個資料集還沒有資料",
		429: "rate_limited：每分鐘的請求太多。quota_exceeded：今天的次數已用完。" +
			"traffic_exceeded：今天的流量已用完。Retry-After 標頭是建議的等待秒數",
	}
	out := obj{}
	for _, c := range codes {
		r := obj{"description": text[c], "content": obj{"application/json": obj{"schema": ref("Error")}}}
		if c == 429 {
			r["headers"] = obj{"Retry-After": obj{"description": "建議等待的秒數", "schema": obj{"type": "integer"}}}
		}
		out[fmt.Sprint(c)] = r
	}
	return out
}

var rateHeaders = obj{
	"X-RateLimit-Limit":     obj{"description": "每天可呼叫的次數", "schema": obj{"type": "integer"}},
	"X-RateLimit-Remaining": obj{"description": "今天剩下的次數（臺灣時間午夜重設）", "schema": obj{"type": "integer"}},
	"X-Traffic-Limit":       obj{"description": "每天可用的流量（位元組）", "schema": obj{"type": "integer"}},
	"X-Traffic-Used":        obj{"description": "今天已用的流量（位元組），不含這次回應", "schema": obj{"type": "integer"}},
}

var (
	etagHeader = obj{"description": "回應內容的指紋。下次請求放進 If-None-Match，資料沒變會回應 304",
		"schema": obj{"type": "string"}}
	ifNoneMatch = param("If-None-Match", "header", "上次回應的 ETag。資料沒變時回應 304，不帶內容",
		obj{"type": "string"}, false)
	notModified = obj{"description": "資料沒變，不帶內容。仍算一次呼叫，但不算流量",
		"headers": obj{"ETag": etagHeader}}
)

func withConditional(op obj) obj {
	ps, _ := op["parameters"].([]obj)
	op["parameters"] = append(ps, ifNoneMatch)
	responses := op["responses"].(obj)
	responses["304"] = notModified
	if ok, has := responses["200"].(obj); has {
		if h, has := ok["headers"].(obj); has {
			h["ETag"] = etagHeader
		}
	}
	return op
}

func tableResponse(desc, row, meta string, extraHeaders obj) obj {
	headers := obj{}
	for k, v := range rateHeaders {
		headers[k] = v
	}
	for k, v := range extraHeaders {
		headers[k] = v
	}
	return obj{
		"description": desc,
		"headers":     headers,
		"content": obj{
			"application/json": obj{"schema": envelope(obj{"type": "array", "items": ref(row)}, meta)},
			"text/csv":         obj{"schema": obj{"type": "string"}},
		},
	}
}

func operation(tag, summary, desc, id string, params []obj, ok obj) obj {
	responses := errorResponses(400, 401, 404, 429)
	responses["200"] = ok
	return withConditional(obj{
		"tags": []string{tag}, "summary": summary, "description": desc, "operationId": id,
		"parameters": params, "responses": responses,
	})
}

func headersWithRate() obj {
	h := obj{}
	for k, v := range rateHeaders {
		h[k] = v
	}
	return h
}

func tagName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(name, "）") {
		if i := strings.LastIndex(name, "（"); i > 0 {
			name = strings.TrimSpace(name[:i])
		}
	}
	return name
}

func schemaName(id string) string {
	var b strings.Builder
	for _, part := range strings.Split(id, "_") {
		if part != "" {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}

func OpenAPI(o SpecOptions) obj {
	schemas := obj{
		"Error": obj{
			"type": "object",
			"properties": obj{"error": obj{
				"type": "object",
				"properties": obj{
					"code":    obj{"type": "string", "description": "錯誤代碼，例如 invalid_county"},
					"message": obj{"type": "string", "description": "給人看的說明"},
				},
				"required": []string{"code", "message"},
			}},
			"required": []string{"error"},
		},
		"Source": obj{
			"type": "object",
			"properties": obj{
				"agency":  obj{"type": "string", "description": "資料提供機關"},
				"license": obj{"type": "string", "description": "授權條款"},
				"url":     obj{"type": "string", "description": "來源資料集網址"},
			},
		},
		"TableMeta": obj{
			"type": "object",
			"properties": obj{
				"dataset":          obj{"type": "string"},
				"source":           ref("Source"),
				"status":           obj{"type": "string", "description": "ok、stale（來源超過預期時間沒有更新）、error、pending"},
				"last_observed_at": obj{"type": []string{"string", "null"}, "format": "date-time", "description": "最新一筆資料的時間"},
			},
		},
		"HistoryMeta": obj{
			"allOf": []obj{ref("TableMeta"), {
				"type": "object",
				"properties": obj{
					"from":        obj{"type": "string", "format": "date-time"},
					"to":          obj{"type": "string", "format": "date-time"},
					"next_cursor": obj{"type": "string", "description": "還有下一頁時才有。放進 cursor 參數取得下一頁"},
				},
			}},
		},
		"Empty": obj{"type": "object"},
		"Token": obj{
			"type": "object",
			"properties": obj{
				"access_token": obj{"type": "string", "description": "放在 Authorization: Bearer 標頭"},
				"token_type":   obj{"type": "string", "enum": []string{"Bearer"}},
				"expires_in":   obj{"type": "integer", "description": "幾秒後過期"},
			},
			"required": []string{"access_token", "token_type", "expires_in"},
		},
		"OAuthError": obj{
			"type": "object",
			"properties": obj{
				"error":             obj{"type": "string", "description": "invalid_request、unsupported_grant_type、invalid_client 或 server_error"},
				"error_description": obj{"type": "string", "description": "給人看的說明"},
			},
			"required": []string{"error"},
		},
		"DownloadIndex": obj{
			"type": "object",
			"properties": obj{
				"dataset":     obj{"type": "string"},
				"description": obj{"type": "string"},
				"license":     obj{"type": "string"},
				"source":      obj{"type": "string"},
				"fields":      obj{"type": "array", "items": ref("Field")},
				"files": obj{"type": "array", "items": obj{
					"type": "object",
					"properties": obj{
						"month":        obj{"type": "string", "examples": []string{"2026-10"}},
						"file":         obj{"type": "string", "examples": []string{"2026-10.csv.gz"}},
						"rows":         obj{"type": "integer"},
						"bytes":        obj{"type": "integer"},
						"sha256":       obj{"type": "string"},
						"generated_at": obj{"type": "string", "format": "date-time"},
					},
				}},
				"updated_at": obj{"type": "string", "format": "date-time"},
			},
		},
		"DatasetSummary": obj{
			"type": "object",
			"properties": obj{
				"id": obj{"type": "string"}, "name": obj{"type": "string"}, "description": obj{"type": "string"},
				"agency": obj{"type": "string"}, "license": obj{"type": "string"}, "source_url": obj{"type": "string"},
				"status":              obj{"type": "string"},
				"update_freq_seconds": obj{"type": "integer", "description": "來源的更新頻率（秒）"},
				"last_observed_at":    obj{"type": []string{"string", "null"}, "format": "date-time"},
				"has_sites":           obj{"type": "boolean", "description": "是否有 /sites（測站、水庫、機組…）"},
				"has_county":          obj{"type": "boolean", "description": "是否可以用 county 篩選"},
			},
		},
		"Field": obj{
			"type": "object",
			"properties": obj{
				"name": obj{"type": "string"}, "type": obj{"type": "string"},
				"unit": obj{"type": "string"}, "description": obj{"type": "string"},
			},
		},
	}

	oauthError := func(desc string) obj {
		return obj{"description": desc, "content": obj{"application/json": obj{"schema": ref("OAuthError")}}}
	}
	paths := obj{
		"/auth/token": obj{"post": obj{
			"tags": []string{"驗證"}, "summary": "取得 Access Token", "operationId": "getToken",
			"description": fmt.Sprintf("OAuth 2.0 Client Credentials。用 Client Id 與 Client Secret 換取 Access Token，有效 %d 小時。"+
				"請重複使用同一個 token，過期再重新取得。這個端點不計入額度。Client Id 與 Secret 也可以用 HTTP Basic 傳送。", o.TokenHours),
			"security": []obj{},
			"requestBody": obj{"required": true, "content": obj{"application/x-www-form-urlencoded": obj{"schema": obj{
				"type": "object",
				"properties": obj{
					"grant_type":    obj{"type": "string", "enum": []string{"client_credentials"}},
					"client_id":     obj{"type": "string", "description": "在「我的 API 憑證」建立的 Client Id"},
					"client_secret": obj{"type": "string", "description": "建立憑證時顯示的 Client Secret"},
				},
				"required": []string{"grant_type"},
			}}}},
			"responses": obj{
				"200": obj{"description": "Access Token", "content": obj{"application/json": obj{"schema": ref("Token")}}},
				"400": oauthError("invalid_request：不是表單格式。unsupported_grant_type：grant_type 不是 client_credentials"),
				"401": oauthError("invalid_client：Client Id 或 Secret 錯誤，或憑證已停用"),
				"429": obj{"description": "請求太頻繁，請稍後再試", "content": obj{"application/json": obj{"schema": ref("Error")}},
					"headers": obj{"Retry-After": obj{"description": "建議等待的秒數", "schema": obj{"type": "integer"}}}},
			},
		}},
		"/datasets": obj{"get": withConditional(obj{
			"tags": []string{"資料集清單"}, "summary": "列出所有資料集", "operationId": "listDatasets",
			"description": "所有資料集的名稱、來源機關、更新頻率與目前狀態。",
			"responses": func() obj {
				r := errorResponses(401, 429)
				r["200"] = obj{"description": "資料集清單", "headers": headersWithRate(), "content": obj{"application/json": obj{
					"schema": envelope(obj{"type": "array", "items": ref("DatasetSummary")}, "Empty"),
				}}}
				return r
			}(),
		})},
	}
	tags := []obj{
		{"name": "驗證", "description": "先用 Client Id 與 Client Secret 換取 Access Token。"},
		{"name": "資料集清單", "description": "先從這裡看有哪些資料集。"},
	}
	downloadsServer := []obj{{"url": "/", "description": "大量下載不在 /api/v1 底下"}}

	for i := range Datasets {
		d := &Datasets[i]
		tag := d.ID
		if name := tagName(o.Names[d.ID]); name != "" {
			tag = name + "（" + d.ID + "）"
		}
		tags = append(tags, obj{"name": tag, "description": d.Description})
		row := schemaName(d.ID)
		schemas[row] = rowSchema(d.Fields)
		latestRow := row
		if d.LatestFields != nil {
			latestRow = row + "Now"
			schemas[latestRow] = rowSchema(d.LatestFields)
		}

		base := "/datasets/" + d.ID
		var filters []obj
		if d.HasCounty {
			filters = append(filters, countyParam)
		}
		if d.HasSite {
			filters = append(filters, param("site", "query", "觀測對象代碼（測站、水庫、機組…），從 /sites 取得",
				obj{"type": "string", "pattern": `^[A-Za-z0-9-]{1,40}$`}, false))
		}
		withFormat := func(ps ...obj) []obj { return append(append(append([]obj{}, filters...), ps...), formatParam) }

		paths[base] = obj{"get": withConditional(obj{
			"tags": []string{tag}, "summary": "資料集說明與欄位", "operationId": "describe" + row,
			"description": "來源、欄位說明、歷史查詢的分頁大小與大量下載的位置。",
			"responses": func() obj {
				r := errorResponses(401, 404, 429)
				r["200"] = obj{"description": "資料集說明", "headers": headersWithRate(), "content": obj{"application/json": obj{"schema": envelope(obj{
					"type": "object",
					"properties": obj{
						"dataset":       ref("DatasetSummary"),
						"fields":        obj{"type": "array", "items": ref("Field"), "description": "history 的欄位"},
						"latest_fields": obj{"type": "array", "items": ref("Field"), "description": "latest 的欄位"},
						"site_fields":   obj{"type": "array", "items": ref("Field")},
						"latest_paged":  obj{"type": "boolean", "description": "latest 是否分頁（名冊類的資料集）"},
						"has_history":   obj{"type": "boolean"},
						"history":       obj{"type": "object", "properties": obj{"page_size": obj{"type": "integer"}, "max_range_days": obj{"type": "integer"}}},
						"downloads":     obj{"type": "string", "description": "大量下載的檔案清單位置"},
					},
				}, "Empty")}}}
				return r
			}(),
		})}
		if d.HasSite {
			siteRow := row + "Site"
			schemas[siteRow] = rowSchema(d.SiteFields)
			var siteParams []obj
			if d.HasCounty {
				siteParams = append(siteParams, countyParam)
			}
			paths[base+"/sites"] = obj{"get": operation(tag, "觀測對象清單",
				"這個資料集的測站、水庫或機組，以及它們的代碼與位置。", "sites"+row,
				append(siteParams, formatParam), tableResponse("觀測對象", siteRow, "TableMeta", nil))}
		}
		if d.LatestPaged {
			paths[base+"/latest"] = obj{"get": operation(tag, "最新資料",
				fmt.Sprintf("目前的完整清單，每頁最多 %d 筆。回應有 next_cursor 時，把它放進 cursor 參數取得下一頁。", PageSize),
				"latest"+row,
				withFormat(param("cursor", "query", "上一頁回傳的 next_cursor", obj{"type": "string"}, false)),
				tableResponse("最新資料", latestRow, "HistoryMeta", obj{
					"X-Next-Cursor": obj{"description": "下一頁的 cursor（和 meta.next_cursor 相同）", "schema": obj{"type": "string"}},
				}))}
		} else {
			paths[base+"/latest"] = obj{"get": operation(tag, "最新資料",
				"每個觀測對象最新的一筆資料。", "latest"+row,
				withFormat(), tableResponse("最新資料", latestRow, "TableMeta", nil))}
		}
		if !d.HasHistory() {
			continue
		}
		paths[base+"/history"] = obj{"get": operation(tag, "歷史資料",
			fmt.Sprintf("指定時間範圍的資料，依時間排序，每頁最多 %d 筆。一次最多查 %d 天。不填 from、to 時為最近 %d 小時。"+
				"回應有 next_cursor 時，把它放進 cursor 參數取得下一頁。更長的期間請用每月的大量下載。",
				PageSize, int(MaxRange.Hours()/24), int(DefaultRange.Hours())),
			"history"+row,
			withFormat(
				param("from", "query", "開始時間（含），RFC 3339（例如 2026-10-01T00:00:00+08:00）或日期（2026-10-01，臺灣時間零點）",
					obj{"type": "string", "examples": []string{"2026-10-01"}}, false),
				param("to", "query", "結束時間（不含），格式同 from。不填為現在",
					obj{"type": "string", "examples": []string{"2026-10-02"}}, false),
				param("cursor", "query", "上一頁回傳的 next_cursor", obj{"type": "string"}, false),
			),
			tableResponse("歷史資料", row, "HistoryMeta", obj{
				"X-Next-Cursor": obj{"description": "下一頁的 cursor（和 meta.next_cursor 相同）", "schema": obj{"type": "string"}},
			}))}

		if !o.Downloads || d.Months == "" {
			continue
		}
		paths["/downloads/"+d.ID+"/index.json"] = obj{"get": obj{
			"tags": []string{tag}, "summary": "大量下載：檔案清單", "operationId": "downloadIndex" + row,
			"description": "每個已經結束的月份一個 .csv.gz 檔，含筆數、大小與 SHA-256。每天 03:30 更新。",
			"servers":     downloadsServer,
			"responses": func() obj {
				r := errorResponses(401, 404, 429)
				r["200"] = obj{"description": "檔案清單", "headers": rateHeaders, "content": obj{"application/json": obj{"schema": ref("DownloadIndex")}}}
				return r
			}(),
		}}
		paths["/downloads/"+d.ID+"/{month}.csv.gz"] = obj{"get": obj{
			"tags": []string{tag}, "summary": "大量下載：一個月的資料", "operationId": "download" + row,
			"description": "一個月的完整資料，gzip 壓縮的 CSV（UTF-8 含 BOM，時間為臺灣時間）。一個檔案算一次請求。",
			"servers":     downloadsServer,
			"parameters": []obj{param("month", "path", "月份，例如 2026-09",
				obj{"type": "string", "pattern": `^\d{4}-\d{2}$`, "examples": []string{"2026-09"}}, true)},
			"responses": func() obj {
				r := errorResponses(401, 404, 429)
				r["200"] = obj{"description": "gzip 壓縮的 CSV", "headers": rateHeaders, "content": obj{"application/gzip": obj{"schema": obj{"type": "string", "format": "binary"}}}}
				return r
			}(),
		}}
	}

	return obj{
		"openapi": "3.1.0",
		"info": obj{
			"title":   "dashboard.tw 開放資料 API",
			"version": o.Version,
			"description": fmt.Sprintf("dashboard.tw 收集的所有政府開放資料（天氣與防災、環境、民生、交通、人口與社會），提供固定格式的最新資料、歷史資料與每月大量下載。\n\n"+
				"- **驗證**：OAuth 2.0 Client Credentials。登入後在「我的 API 憑證」建立 Client Id 與 Client Secret，"+
				"呼叫 `POST /auth/token` 換取 Access Token（有效 %d 小時），放在 `Authorization: Bearer <Access Token>` 標頭\n"+
				"- **額度**：每天 %d 次、%d MB 流量，每分鐘 %d 次，臺灣時間午夜重設。流量以回應內容的大小計算。"+
				"應用上架到應用活化後，提高為每天 %d 次、%d MB 流量，每分鐘 %d 次\n"+
				"- **ETag**：帶上 `If-None-Match`，資料沒變會回應 304，不帶內容。304 仍算一次，但不算流量\n"+
				"- **時間**：一律是臺灣時間（+08:00）\n"+
				"- **授權**：資料依各來源機關的授權條款（多為政府資料開放授權條款－第 1 版），使用時請標示來源\n\n"+
				"這份文件由網站程式自動產生，和 API 的實際行為一致。", o.TokenHours, o.PerDay, o.MBPerDay, o.PerMinute,
				o.PartnerPerDay, o.PartnerMBPerDay, o.PartnerPerMinute),
			"license": obj{"name": "政府資料開放授權條款－第 1 版", "url": "https://data.gov.tw/license"},
		},
		"servers":  []obj{{"url": "/api/v1"}},
		"security": []obj{{"oauth2": []string{}}},
		"tags":     tags,
		"paths":    paths,
		"components": obj{
			"securitySchemes": obj{"oauth2": obj{
				"type":        "oauth2",
				"description": "登入後在「我的 API 憑證」建立 Client Id 與 Client Secret，換取 Access Token 後放在 Authorization: Bearer 標頭。",
				"flows": obj{"clientCredentials": obj{
					"tokenUrl": "/api/v1/auth/token",
					"scopes":   obj{},
				}},
			}},
			"schemas": schemas,
		},
	}
}
