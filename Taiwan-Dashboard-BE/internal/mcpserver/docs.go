package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *server) addDocs(srv *mcp.Server, ro func(string) *mcp.ToolAnnotations) {
	if s.OpenAPI == nil {
		return
	}
	mcp.AddTool(srv, &mcp.Tool{Name: "get_api_overview", Annotations: ro("API overview"),
		Description: "How to use the Taiwan Dashboard open data REST API: base URL, getting an access token, quotas, ETag, pagination, errors and links to the full docs. Free."}, s.apiOverview)
	mcp.AddTool(srv, &mcp.Tool{Name: "list_api_endpoints", Annotations: ro("List API endpoints"),
		Description: "List REST API endpoints with method, full URL and summary. Filter by dataset id (e.g. aqi_obs) or keyword. Free."}, s.listEndpoints)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_api_endpoint", Annotations: ro("API endpoint details"),
		Description: "Full details of one REST API endpoint: parameters, request body, responses and error codes, with referenced schemas inlined. Free."}, s.getEndpoint)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_openapi_spec", Annotations: ro("OpenAPI spec"),
		Description: "Get an OpenAPI 3 spec for generating API clients. With a dataset id, returns only that dataset's endpoints plus authentication. Without one, returns the full spec URL and the dataset list, because the full spec is large. Free."}, s.openAPISpec)
	srv.AddResource(&mcp.Resource{
		URI: s.Site + "/api/v1/openapi.json", Name: "openapi", Title: "Taiwan Dashboard OpenAPI spec",
		Description: "The full OpenAPI 3 spec of the open data REST API.", MIMEType: "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		raw, err := json.Marshal(s.spec(ctx))
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: string(raw)}}}, nil
	})
}

func (s *server) baseURL() string { return s.Site + "/api/v1" }

func (s *server) spec(ctx context.Context) map[string]any {
	sp := s.OpenAPI(ctx)
	sp["servers"] = []any{map[string]any{"url": s.baseURL()}}
	return sp
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

type overviewIn struct{}

func (s *server) apiOverview(ctx context.Context, _ *mcp.CallToolRequest, _ overviewIn) (*mcp.CallToolResult, any, error) {
	sp := s.spec(ctx)
	info := asMap(sp["info"])
	var tags []string
	for _, t := range asSlice(sp["tags"]) {
		if n, ok := asMap(t)["name"].(string); ok {
			tags = append(tags, n)
		}
	}
	return nil, map[string]any{
		"title": info["title"], "version": info["version"], "base_url": s.baseURL(),
		"description": info["description"],
		"authentication": map[string]any{
			"flow": "OAuth 2.0 client credentials",
			"steps": []string{
				"Sign in at " + s.Site + " with GitHub or Google and create an API credential at " + s.Site + "/me/api-key to get a Client ID and Client Secret.",
				"POST " + s.baseURL() + "/auth/token with grant_type=client_credentials, client_id and client_secret (form-encoded or HTTP Basic) to get an access token.",
				"Send Authorization: Bearer <access token> on every data request. Reuse the token until it expires.",
			},
		},
		"quota_note":   "Data requests count toward a shared daily quota per account. Remaining quota is in the X-RateLimit-Remaining header.",
		"docs":         s.Site + "/developers",
		"openapi":      s.baseURL() + "/openapi.json",
		"dataset_list": s.Site + "/developers/datasets",
		"llms_txt":     s.Site + "/llms.txt",
		"tags":         tags,
		"next":         "Use list_api_endpoints to find an endpoint, then get_api_endpoint for its parameters and response format.",
	}, nil
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	if s != nil {
		return s
	}
	if m, ok := v.([]map[string]any); ok {
		out := make([]any, len(m))
		for i := range m {
			out[i] = m[i]
		}
		return out
	}
	return nil
}

var httpMethods = []string{"get", "post", "put", "patch", "delete"}

type listEndpointsIn struct {
	Dataset string `json:"dataset,omitempty" jsonschema:"only endpoints of this dataset id, e.g. aqi_obs"`
	Keyword string `json:"keyword,omitempty" jsonschema:"match the path, summary or tag, in English or Chinese"`
}

type endpointItem struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	URL     string `json:"url"`
	Summary string `json:"summary"`
	Tag     string `json:"tag,omitempty"`
}

func (s *server) endpoints(ctx context.Context) ([]endpointItem, map[string]any) {
	sp := s.spec(ctx)
	paths := asMap(sp["paths"])
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []endpointItem
	for _, p := range keys {
		item := asMap(paths[p])
		for _, m := range httpMethods {
			op := asMap(item[m])
			if op == nil {
				continue
			}
			e := endpointItem{Method: strings.ToUpper(m), Path: p, URL: s.baseURL() + p}
			e.Summary, _ = op["summary"].(string)
			if tags := asSlice(op["tags"]); len(tags) > 0 {
				e.Tag, _ = tags[0].(string)
			}
			out = append(out, e)
		}
	}
	return out, sp
}

func (s *server) listEndpoints(ctx context.Context, _ *mcp.CallToolRequest, in listEndpointsIn) (*mcp.CallToolResult, any, error) {
	all, _ := s.endpoints(ctx)
	ds := strings.TrimSpace(in.Dataset)
	words := expandQuery(in.Keyword)
	if strings.TrimSpace(in.Keyword) == "" {
		words = nil
	}
	out := []endpointItem{}
	for _, e := range all {
		if ds != "" && !strings.Contains(e.Path+"/", "/"+ds+"/") && !strings.Contains(e.Tag, "（"+ds+"）") {
			continue
		}
		if len(words) > 0 && score(words, e.Path, e.Summary, e.Tag) == 0 {
			continue
		}
		out = append(out, e)
	}
	res := map[string]any{"base_url": s.baseURL(), "count": len(out), "endpoints": out}
	if ds == "" && len(words) == 0 {
		res["hint"] = "Every dataset has /datasets/{id}, /datasets/{id}/latest and usually /datasets/{id}/history. Pass dataset to narrow the list."
	}
	return nil, res, nil
}

type endpointIn struct {
	Path   string `json:"path" jsonschema:"endpoint path or full URL, e.g. /datasets/aqi_obs/latest"`
	Method string `json:"method,omitempty" jsonschema:"HTTP method, default GET"`
}

func (s *server) getEndpoint(ctx context.Context, _ *mcp.CallToolRequest, in endpointIn) (*mcp.CallToolResult, any, error) {
	p := strings.TrimSpace(in.Path)
	if u, err := url.Parse(p); err == nil && u.Host != "" {
		p = u.Path
	}
	p = strings.TrimPrefix(p, "/api/v1")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	m := strings.ToLower(strings.TrimSpace(in.Method))
	if m == "" {
		m = "get"
	}
	sp := s.spec(ctx)
	item := asMap(asMap(sp["paths"])[p])
	if item == nil {
		return nil, nil, fmt.Errorf("no endpoint %s; use list_api_endpoints", p)
	}
	op := asMap(item[m])
	if op == nil {
		return nil, nil, fmt.Errorf("%s does not support %s", p, strings.ToUpper(m))
	}
	resolved := resolveRefs(sp, op, 0, map[string]bool{})
	return nil, map[string]any{"method": strings.ToUpper(m), "path": p, "url": s.baseURL() + p,
		"operation": resolved, "auth": "Authorization: Bearer <access token>, except POST /auth/token and the embed endpoint"}, nil
}

func resolveRefs(root map[string]any, v any, depth int, seen map[string]bool) any {
	if depth > 12 {
		return v
	}
	switch x := v.(type) {
	case map[string]any:
		if ref, ok := x["$ref"].(string); ok && len(x) == 1 {
			if seen[ref] {
				return map[string]any{"$ref": ref}
			}
			target := lookupRef(root, ref)
			if target == nil {
				return x
			}
			seen[ref] = true
			out := resolveRefs(root, target, depth+1, seen)
			delete(seen, ref)
			return out
		}
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = resolveRefs(root, val, depth+1, seen)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = resolveRefs(root, val, depth+1, seen)
		}
		return out
	case []map[string]any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = resolveRefs(root, val, depth+1, seen)
		}
		return out
	}
	return v
}

func lookupRef(root map[string]any, ref string) any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	var cur any = root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		m := asMap(cur)
		if m == nil {
			return nil
		}
		cur = m[part]
	}
	return cur
}

type specIn struct {
	Dataset string `json:"dataset,omitempty" jsonschema:"dataset id, e.g. aqi_obs; leave empty for the spec URL and dataset list"`
}

func (s *server) openAPISpec(ctx context.Context, _ *mcp.CallToolRequest, in specIn) (*mcp.CallToolResult, any, error) {
	sp := s.spec(ctx)
	ds := strings.TrimSpace(in.Dataset)
	if ds == "" {
		list, err := s.Open.List(ctx)
		if err != nil {
			return nil, nil, err
		}
		ids := make([]map[string]string, 0, len(list))
		for _, x := range list {
			ids = append(ids, map[string]string{"id": x.ID, "name": x.Name})
		}
		return nil, map[string]any{
			"full_spec_url": s.baseURL() + "/openapi.json",
			"note":          "The full spec covers every dataset and is large. Call get_openapi_spec with a dataset id for a smaller spec, or download the full one from full_spec_url.",
			"datasets":      ids,
		}, nil
	}
	if !datasetRe.MatchString(ds) {
		return nil, nil, errors.New("invalid dataset id; see list_datasets")
	}
	paths := asMap(sp["paths"])
	sub := map[string]any{}
	for p, item := range paths {
		if p == "/auth/token" || p == "/datasets" || p == "/datasets/"+ds || strings.HasPrefix(p, "/datasets/"+ds+"/") ||
			strings.HasPrefix(p, "/downloads/"+ds+"/") {
			sub[p] = item
		}
	}
	if len(sub) <= 2 {
		return nil, nil, fmt.Errorf("dataset %s has no endpoints; see list_datasets", ds)
	}
	used := map[string]bool{}
	collectRefs(sp, sub, used)
	comps := asMap(sp["components"])
	outComps := map[string]any{}
	for kind, group := range comps {
		g := asMap(group)
		if kind == "securitySchemes" {
			outComps[kind] = g
			continue
		}
		picked := map[string]any{}
		for name, v := range g {
			if used["#/components/"+kind+"/"+name] {
				picked[name] = v
			}
		}
		if len(picked) > 0 {
			outComps[kind] = picked
		}
	}
	out := map[string]any{"openapi": sp["openapi"], "info": sp["info"], "servers": sp["servers"],
		"security": sp["security"], "paths": sub, "components": outComps}
	return nil, out, nil
}

func collectRefs(root map[string]any, v any, used map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		if ref, ok := x["$ref"].(string); ok {
			if !used[ref] {
				used[ref] = true
				collectRefs(root, lookupRef(root, ref), used)
			}
			return
		}
		for _, val := range x {
			collectRefs(root, val, used)
		}
	case []any:
		for _, val := range x {
			collectRefs(root, val, used)
		}
	case []map[string]any:
		for _, val := range x {
			collectRefs(root, val, used)
		}
	}
}
