package opendata

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func spec(t *testing.T, o SpecOptions) map[string]any {
	t.Helper()
	b, err := json.Marshal(OpenAPI(o))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func walk(v any, fn func(map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		fn(x)
		for _, c := range x {
			walk(c, fn)
		}
	case []any:
		for _, c := range x {
			walk(c, fn)
		}
	}
}

var pathParam = regexp.MustCompile(`\{([a-z_]+)\}`)

func TestOpenAPIIsConsistent(t *testing.T) {
	doc := spec(t, SpecOptions{Version: "test", PerDay: 100, PerMinute: 5, MBPerDay: 200, TokenHours: 24, Downloads: true,
		Names: map[string]string{"aqi_obs": "空氣品質指標（每小時）"}})
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v", doc["openapi"])
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)

	walk(doc, func(m map[string]any) {
		if r, ok := m["$ref"].(string); ok {
			name := strings.TrimPrefix(r, "#/components/schemas/")
			if _, ok := schemas[name]; !ok {
				t.Errorf("dangling $ref %s", r)
			}
		}
	})

	tags := map[string]bool{}
	for _, tg := range doc["tags"].([]any) {
		tags[tg.(map[string]any)["name"].(string)] = true
	}
	ids := map[string]bool{}
	paths := doc["paths"].(map[string]any)
	for path, item := range paths {
		var op map[string]any
		for _, m := range []string{"get", "post"} {
			if o, ok := item.(map[string]any)[m].(map[string]any); ok {
				op = o
			}
		}
		if op == nil {
			t.Errorf("%s has no operation", path)
			continue
		}
		id := op["operationId"].(string)
		if ids[id] {
			t.Errorf("duplicate operationId %s", id)
		}
		ids[id] = true
		for _, tg := range op["tags"].([]any) {
			if !tags[tg.(string)] {
				t.Errorf("%s uses undeclared tag %v", path, tg)
			}
		}
		declared := map[string]bool{}
		if ps, ok := op["parameters"].([]any); ok {
			for _, p := range ps {
				pm := p.(map[string]any)
				if pm["in"] == "path" {
					declared[pm["name"].(string)] = true
				}
			}
		}
		for _, m := range pathParam.FindAllStringSubmatch(path, -1) {
			if !declared[m[1]] {
				t.Errorf("%s: path parameter %s is not declared", path, m[1])
			}
			delete(declared, m[1])
		}
		if len(declared) > 0 {
			t.Errorf("%s declares unused path parameters %v", path, declared)
		}
		if _, ok := op["responses"].(map[string]any)["200"]; !ok {
			t.Errorf("%s has no 200 response", path)
		}
	}

	seen := map[string]bool{}
	topicIDs := map[string]bool{}
	for _, tp := range Topics {
		topicIDs[tp.ID] = true
	}
	for _, d := range Datasets {
		if seen[d.ID] {
			t.Errorf("dataset %s is listed twice", d.ID)
		}
		seen[d.ID] = true
		if !topicIDs[TopicOf(d.ID)] {
			t.Errorf("%s has unknown topic %q", d.ID, TopicOf(d.ID))
		}
	}

	for _, d := range Datasets {
		latest, ok := paths["/datasets/"+d.ID+"/latest"]
		if !ok {
			t.Errorf("%s has no latest path", d.ID)
			continue
		}
		params := map[string]bool{}
		for _, p := range latest.(map[string]any)["get"].(map[string]any)["parameters"].([]any) {
			params[p.(map[string]any)["name"].(string)] = true
		}
		if params["county"] != d.HasCounty || params["site"] != d.HasSite || !params["format"] || params["cursor"] != d.LatestPaged {
			t.Errorf("%s latest params = %v", d.ID, params)
		}
		if _, ok := paths["/datasets/"+d.ID+"/sites"]; ok != d.HasSite {
			t.Errorf("%s sites path = %v, has sites %v", d.ID, ok, d.HasSite)
		}
		if _, ok := paths["/datasets/"+d.ID+"/history"]; ok != d.HasHistory() {
			t.Errorf("%s history path = %v, has history %v", d.ID, ok, d.HasHistory())
		}
		if _, ok := paths["/downloads/"+d.ID+"/{month}.csv.gz"]; ok != (d.Months != "") {
			t.Errorf("%s download path = %v, has months %v", d.ID, ok, d.Months != "")
		}
		if d.Months != "" && !d.HasHistory() {
			t.Errorf("%s has monthly files but no history query to write them", d.ID)
		}
		row := schemas[schemaName(d.ID)].(map[string]any)["properties"].(map[string]any)
		if len(row) != len(d.Fields) {
			t.Errorf("%s row schema has %d properties, want %d", d.ID, len(row), len(d.Fields))
		}
		for _, f := range append(append(append([]Field{}, d.Fields...), d.LatestFields...), d.SiteFields...) {
			switch f.Type {
			case "string", "integer", "number", "boolean", "timestamp", "date":
			default:
				t.Errorf("%s.%s has unknown type %q", d.ID, f.Name, f.Type)
			}
		}
		if TopicOf(d.ID) == "" {
			t.Errorf("%s has no topic", d.ID)
		}
	}

	if !tags["空氣品質指標（aqi_obs）"] {
		t.Error("aqi_obs tag does not use its display name")
	}
	if desc := doc["info"].(map[string]any)["description"].(string); !strings.Contains(desc, "每天 100 次、200 MB 流量，每分鐘 5 次") {
		t.Errorf("quota missing from description")
	}
}

func TestOpenAPIAuth(t *testing.T) {
	doc := spec(t, SpecOptions{TokenHours: 24})
	scheme := doc["components"].(map[string]any)["securitySchemes"].(map[string]any)["oauth2"].(map[string]any)
	flow := scheme["flows"].(map[string]any)["clientCredentials"].(map[string]any)
	if scheme["type"] != "oauth2" || flow["tokenUrl"] != "/api/v1/auth/token" {
		t.Errorf("security scheme = %v", scheme)
	}
	token := doc["paths"].(map[string]any)["/auth/token"].(map[string]any)["post"].(map[string]any)
	if s, ok := token["security"].([]any); !ok || len(s) != 0 {
		t.Errorf("token endpoint should need no token, security = %v", token["security"])
	}
	latest := doc["paths"].(map[string]any)["/datasets/aqi_obs/latest"].(map[string]any)["get"].(map[string]any)
	if _, ok := latest["responses"].(map[string]any)["304"]; !ok {
		t.Error("latest has no 304 response")
	}
	headers := latest["responses"].(map[string]any)["200"].(map[string]any)["headers"].(map[string]any)
	for _, h := range []string{"ETag", "X-Traffic-Used", "X-RateLimit-Remaining"} {
		if _, ok := headers[h]; !ok {
			t.Errorf("latest 200 lacks %s header", h)
		}
	}
	if strings.Contains(fmt.Sprint(doc), "dtw_") {
		t.Error("spec still mentions the old dtw_ keys")
	}
}

func TestOpenAPIWithoutDownloads(t *testing.T) {
	for path := range spec(t, SpecOptions{})["paths"].(map[string]any) {
		if strings.HasPrefix(path, "/downloads/") {
			t.Errorf("%s documented although downloads are off", path)
		}
	}
}
