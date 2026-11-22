package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ardEntry struct {
	Identifier            string   `json:"identifier"`
	DisplayName           string   `json:"displayName"`
	Type                  string   `json:"type"`
	URL                   string   `json:"url"`
	Description           string   `json:"description"`
	RepresentativeQueries []string `json:"representativeQueries"`
	Tags                  []string `json:"tags"`
}

const ardPath = "/.well-known/ard.json"

func ardManifest(base string) gin.HandlerFunc {
	body, _ := json.MarshalIndent(map[string]any{"entries": []ardEntry{
		{
			Identifier:  "urn:air:dashboard.tw:opendata:api",
			DisplayName: "臺灣儀表板開放資料 API",
			Type:        "application/vnd.oai.openapi+json",
			URL:         base + "/api/v1/openapi.json",
			Description: "全臺各縣市的天氣、空氣品質、水情、電力、交通與防災資料，提供即時與歷史查詢。需要免費申請的 API 憑證。",
			RepresentativeQueries: []string{
				"臺灣各縣市目前的空氣品質",
				"水庫蓄水率的歷史資料",
				"臺灣即時發電結構與用電",
				"各縣市的雨量與氣溫",
			},
			Tags: []string{"taiwan", "open-data", "weather", "air-quality", "power", "reservoir"},
		},
		{
			Identifier:  "urn:air:dashboard.tw:opendata:catalog",
			DisplayName: "臺灣儀表板資料集清單",
			Type:        "application/json",
			URL:         base + "/api/v1/catalog",
			Description: "開放資料 API 提供的所有資料集、欄位與更新頻率。",
			RepresentativeQueries: []string{
				"臺灣儀表板有哪些資料集",
				"開放資料 API 的欄位說明",
			},
			Tags: []string{"taiwan", "open-data", "catalog"},
		},
	}}, "", "  ")
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=3600")
		c.Header("Access-Control-Allow-Origin", "*")
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	}
}
