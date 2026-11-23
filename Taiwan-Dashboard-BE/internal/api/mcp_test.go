package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/screen"
	"dashboard.tw/backend/internal/testdb"
)

type headerTransport struct {
	value string
	base  http.RoundTripper
}

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = "dashboard.tw"
	r.Header.Set("Authorization", "Bearer "+h.value)
	return h.base.RoundTrip(r)
}

func TestMCP(t *testing.T) {
	e := newOpenEnv(t)
	scr := screen.New(e.store.DB, &component.Executor{DB: testdb.Reader(t, e.db)})
	e.router = NewRouter(Deps{DB: fakePinger{}, OpenData: &opendata.Service{DB: e.store.DB}, Screen: scr,
		RatePerMinute: 6000, RateBurst: 1000,
		Auth: AuthDeps{Store: e.store, Providers: auth.Providers(false, false), Origins: []string{testOrigin}}})
	srv := httptest.NewServer(e.router)
	defer srv.Close()

	res, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(res.Header.Get("WWW-Authenticate"), "Bearer") {
		t.Fatalf("no token: %d %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}

	a := e.signInAs(t, auth.RoleUser, "active")
	clientID, secret, _ := e.newClient(t, a, "MCP 測試")
	token, _, err := e.store.IssueToken(context.Background(), clientID, secret)
	if err != nil {
		t.Fatal(err)
	}
	withQuota(t, 8, 60)

	for _, cred := range []string{token, clientID + ":" + secret} {
		hc := &http.Client{Transport: headerTransport{value: cred, base: http.DefaultTransport}}
		c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
		sess, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: hc}, nil)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		tools, err := sess.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, tl := range tools.Tools {
			names[tl.Name] = true
			if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
				t.Errorf("%s is not marked read-only", tl.Name)
			}
		}
		for _, n := range []string{"search", "fetch", "list_datasets", "describe_dataset", "get_latest", "get_history", "get_component"} {
			if !names[n] {
				t.Errorf("tool %s missing", n)
			}
		}

		call := func(name string, args map[string]any) map[string]any {
			t.Helper()
			r, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if r.IsError {
				t.Fatalf("%s returned an error: %+v", name, r.Content)
			}
			raw, _ := json.Marshal(r.StructuredContent)
			var out map[string]any
			_ = json.Unmarshal(raw, &out)
			return out
		}
		latest := call("get_latest", map[string]any{"dataset": "aqi_obs", "county": "臺北市", "limit": 5})
		rows, _ := latest["rows"].([]any)
		if len(rows) != 1 || latest["source"] == nil {
			t.Errorf("get_latest = %v", latest)
		}
		hist := call("get_history", map[string]any{"dataset": "aqi_obs", "limit": 50})
		if rows, _ := hist["rows"].([]any); len(rows) != 50 || hist["next_cursor"] == nil {
			t.Errorf("get_history rows = %d, cursor = %v", len(rows), hist["next_cursor"])
		}
		for _, q := range []string{"空氣品質", "air quality", "AQI"} {
			found := call("search", map[string]any{"query": q})
			if results, _ := found["results"].([]any); len(results) == 0 {
				t.Errorf("search %q found nothing: %v", q, found)
			}
		}
		sites := call("list_sites", map[string]any{"dataset": "reservoir_obs", "name": "石門"})
		if list, _ := sites["sites"].([]any); len(list) != 1 || sites["id_field"] != "reservoir_id" {
			t.Errorf("list_sites = %v", sites)
		}
		en := call("get_latest", map[string]any{"dataset": "aqi_obs", "county": "Taipei"})
		if rows, _ := en["rows"].([]any); len(rows) != 1 {
			t.Errorf("county Taipei rows = %v", en["rows"])
		}
		doc := call("fetch", map[string]any{"id": "dataset:aqi_obs"})
		if !strings.Contains(doc["text"].(string), "Latest rows") || doc["url"] == "" {
			t.Errorf("fetch = %v", doc)
		}
		bad, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_latest", Arguments: map[string]any{"dataset": "aqi_obs", "county": "火星"}})
		if err != nil || !bad.IsError {
			t.Errorf("unknown county: err=%v isError=%v", err, bad != nil && bad.IsError)
		}
		sess.Close()
	}

	used, err := e.store.UsedToday(context.Background(), userIDOf(t, e, a))
	if err != nil || used.Requests != 8 || used.Bytes <= 0 {
		t.Errorf("data tools should use the API quota: %+v %v", used, err)
	}

	hc := &http.Client{Transport: headerTransport{value: token, base: http.DefaultTransport}}
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	sess, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: hc}, nil)
	if err != nil {
		t.Fatal(err)
	}
	over, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_latest", Arguments: map[string]any{"dataset": "aqi_obs"}})
	if err != nil || !over.IsError || !strings.Contains(fmt.Sprint(over.Content[0]), "used up") {
		t.Errorf("over quota: err=%v result=%+v", err, over)
	}
	free, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"query": "reservoir"}})
	if err != nil || free.IsError {
		t.Errorf("search should stay free after the quota is used up: err=%v", err)
	}
	doc := func(name string, args map[string]any) map[string]any {
		t.Helper()
		r, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || r.IsError {
			t.Fatalf("%s after the quota is used up: err=%v result=%+v", name, err, r)
		}
		raw, _ := json.Marshal(r.StructuredContent)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return out
	}
	if ov := doc("get_api_overview", map[string]any{}); !strings.HasSuffix(fmt.Sprint(ov["base_url"]), "/api/v1") || ov["authentication"] == nil {
		t.Errorf("overview = %v", ov)
	}
	if eps := doc("list_api_endpoints", map[string]any{"dataset": "aqi_obs"}); fmt.Sprint(eps["count"]) == "0" {
		t.Errorf("no aqi_obs endpoints: %v", eps)
	}
	ep := doc("get_api_endpoint", map[string]any{"path": "https://dashboard.tw/api/v1/datasets/aqi_obs/latest"})
	if raw, _ := json.Marshal(ep); !strings.Contains(string(raw), "pm25") || strings.Contains(string(raw), "#/components/schemas/AqiObs\"") {
		t.Errorf("endpoint schema was not inlined: %.300s", raw)
	}
	spec := doc("get_openapi_spec", map[string]any{"dataset": "aqi_obs"})
	paths, _ := spec["paths"].(map[string]any)
	if paths["/auth/token"] == nil || paths["/datasets/aqi_obs/latest"] == nil || paths["/datasets/power_gen/latest"] != nil {
		t.Errorf("dataset spec paths = %v", paths)
	}
	rr, err := sess.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: testOrigin + "/api/v1/openapi.json"})
	if err != nil || len(rr.Contents) != 1 || !strings.Contains(rr.Contents[0].Text, "\"openapi\"") {
		t.Errorf("openapi resource: %v", err)
	}
	sess.Close()
	var calls int
	if err := e.db.QueryRow(`SELECT COALESCE(sum(calls), 0) FROM app.mcp_usage_daily`).Scan(&calls); err != nil || calls < 10 {
		t.Errorf("mcp usage calls = %d, %v", calls, err)
	}

	hc = &http.Client{Transport: headerTransport{value: clientID + ":wrong", base: http.DefaultTransport}}
	c = mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	if _, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: hc}, nil); err == nil {
		t.Error("a wrong secret was accepted")
	}
}

func userIDOf(t *testing.T, e authEnv, session string) int64 {
	t.Helper()
	u, err := e.store.Lookup(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}
