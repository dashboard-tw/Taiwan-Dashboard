package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/auth"
)

var validApp = map[string]any{
	"category": "web", "name": "空品小幫手", "web_url": "https://example.org/aqi", "description": "每天早上推播住家附近的空品。",
	"topics": []string{"aqi", "weather"}, "show_author": true, "agree_rules": true,
}

func appWith(k string, v any) map[string]any {
	m := map[string]any{}
	for kk, vv := range validApp {
		m[kk] = vv
	}
	m[k] = v
	return m
}

func TestShowcaseApps(t *testing.T) {
	e := newOpenEnv(t)
	a := e.signInAs(t, auth.RoleUser, "active")
	b := e.signInAs(t, auth.RoleUser, "active")
	admin := e.signInAs(t, auth.RoleAdmin, "active")

	for name, body := range map[string]any{
		"not agreed":     appWith("agree_rules", false),
		"no name":        appWith("name", "  "),
		"long name":      appWith("name", strings.Repeat("字", 41)),
		"hidden char":    appWith("name", "a​b"),
		"no description": appWith("description", ""),
		"script url":     appWith("web_url", "javascript:alert(1)"),
		"url with login": appWith("web_url", "https://u:p@example.org"),
		"no link":        appWith("web_url", ""),
		"fake app store": appWith("ios_url", "https://apps.apple.com.evil.example/app"),
		"http play":      appWith("android_url", "http://play.google.com/store/apps/details?id=x"),
		"image by json":  appWith("image", "0123456789abcdef0123456789abcdef.png"),
		"no category":    appWith("category", ""),
		"bad category":   appWith("category", "game"),
		"app no store":   appWith("category", "app"),
		"no topics":      appWith("topics", []string{}),
		"unknown topic":  appWith("topics", []string{"rain"}),
		"unknown field":  appWith("user_id", 1),
	} {
		if w := e.send("POST", "/api/v1/me/apps", a, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, w.Code)
		}
	}

	if w := e.send("POST", "/api/v1/me/apps", a, validApp); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), "client_required") {
		t.Fatalf("apply without a credential: %d %s, want 409 client_required", w.Code, w.Body)
	}
	e.newKey(t, a)

	w := e.send("POST", "/api/v1/me/apps", a, validApp)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w.Code, w.Body)
	}
	mine := decode[[]myApp](t, w)
	if len(mine) != 1 || mine[0].Status != "pending" || mine[0].Published != nil {
		t.Fatalf("after submit: %+v", mine)
	}
	id := strconv.FormatInt(mine[0].ID, 10)

	if got := decode[[]myApp](t, e.send("GET", "/api/v1/me/apps", b, nil)); len(got) != 0 {
		t.Errorf("B sees %d apps", len(got))
	}
	if w := e.send("PUT", "/api/v1/me/apps/"+id, b, validApp); w.Code != http.StatusNotFound {
		t.Errorf("B edits A's app: %d, want 404", w.Code)
	}
	if w := e.send("DELETE", "/api/v1/me/apps/"+id, b, nil); w.Code != http.StatusNotFound {
		t.Errorf("B deletes A's app: %d, want 404", w.Code)
	}

	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", b, map[string]any{"action": "approve"}); w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("user reviews: %d", w.Code)
	}

	tier := func() string {
		uid := e.userOf(t, a)
		got, err := e.store.TierOf(t.Context(), uid)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	public := func() []publicApp { return decode[[]publicApp](t, e.send("GET", "/api/v1/apps", "", nil)) }

	if tier() != auth.TierFree || len(public()) != 0 {
		t.Fatal("a pending app is already public or upgraded")
	}

	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", admin, map[string]any{"action": "reject"}); w.Code != http.StatusBadRequest {
		t.Errorf("reject without a note: %d, want 400", w.Code)
	}
	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", admin, map[string]any{"action": "approve"}); w.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", admin, map[string]any{"action": "approve"}); w.Code != http.StatusConflict {
		t.Errorf("approve twice: %d, want 409", w.Code)
	}
	if tier() != auth.TierPartner {
		t.Errorf("tier after approval = %s", tier())
	}
	list := public()
	if len(list) != 1 || list[0].Name != "空品小幫手" || list[0].Author == nil || list[0].Category != "web" {
		t.Fatalf("public after approval: %+v", list)
	}
	if w := e.send("GET", "/api/v1/apps", "", nil); !strings.Contains(w.Header().Get("Cache-Control"), "public") {
		t.Errorf("public list Cache-Control = %q", w.Header().Get("Cache-Control"))
	}

	edit := appWith("name", "新名字")
	edit["show_author"] = false
	edit["category"] = "innovation"
	if w := e.send("PUT", "/api/v1/me/apps/"+id, a, edit); w.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", w.Code, w.Body)
	}
	if list := public(); len(list) != 1 || list[0].Name != "空品小幫手" || tier() != auth.TierPartner {
		t.Errorf("during review: %+v tier=%s", list, tier())
	}
	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", admin, map[string]any{"action": "reject", "note": "名稱太像官方"}); w.Code != http.StatusNoContent {
		t.Fatalf("reject: %d %s", w.Code, w.Body)
	}
	mine = decode[[]myApp](t, e.send("GET", "/api/v1/me/apps", a, nil))
	if mine[0].Status != "rejected" || mine[0].ReviewNote != "名稱太像官方" || mine[0].Published == nil {
		t.Errorf("after rejecting the edit: %+v", mine[0])
	}
	if w := e.send("PUT", "/api/v1/me/apps/"+id, a, edit); w.Code != http.StatusOK {
		t.Fatal(w.Body)
	}
	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", admin, map[string]any{"action": "approve"}); w.Code != http.StatusNoContent {
		t.Fatal(w.Body)
	}
	if list := public(); len(list) != 1 || list[0].Name != "新名字" || list[0].Author != nil || list[0].Category != "innovation" {
		t.Errorf("after approving the edit: %+v", list)
	}

	admins := decode[[]adminApp](t, e.send("GET", "/api/v1/admin/apps?status=approved", admin, nil))
	if len(admins) != 1 || admins[0].UserID != e.userOf(t, a) {
		t.Errorf("admin list: %+v", admins)
	}

	if w := e.send("POST", "/api/v1/admin/apps/"+id+"/review", admin, map[string]any{"action": "remove", "note": "網站已經關閉"}); w.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", w.Code, w.Body)
	}
	if len(public()) != 0 || tier() != auth.TierFree {
		t.Errorf("after removal: public=%d tier=%s", len(public()), tier())
	}

	for i := 0; i < 5; i++ {
		if w := e.send("POST", "/api/v1/me/apps", a, validApp); w.Code != http.StatusOK {
			t.Fatalf("app %d: %d", i+2, w.Code)
		}
	}
	if got := decode[[]myApp](t, e.send("GET", "/api/v1/me/apps", a, nil)); len(got) != 6 {
		t.Errorf("apps = %d, want 6", len(got))
	}
	if w := e.send("DELETE", "/api/v1/me/apps/"+id, a, nil); w.Code != http.StatusNoContent {
		t.Errorf("withdraw: %d", w.Code)
	}
}

func TestPartnerQuotaHeaders(t *testing.T) {
	e := newOpenEnv(t)
	a := e.signInAs(t, auth.RoleUser, "active")
	key := e.newKey(t, a)
	if w := e.withKey("/api/v1/datasets", key); w.Header().Get("X-RateLimit-Limit") != strconv.Itoa(auth.Tiers[auth.TierFree].PerDay) {
		t.Fatalf("free limit header = %q", w.Header().Get("X-RateLimit-Limit"))
	}
	if _, err := e.db.Exec(`
		INSERT INTO app.showcase_app (user_id, category, name, web_url, description, topics, status, published, published_at)
		VALUES ($1, 'web', 'x', 'https://example.org', 'x', '{aqi}', 'approved', '{}'::jsonb, now())`, e.userOf(t, a)); err != nil {
		t.Fatal(err)
	}
	if w := e.withKey("/api/v1/datasets", key); w.Header().Get("X-RateLimit-Limit") != strconv.Itoa(auth.Tiers[auth.TierPartner].PerDay) {
		t.Errorf("partner limit header = %q", w.Header().Get("X-RateLimit-Limit"))
	}
}

func (e authEnv) userOf(t *testing.T, session string) int64 {
	t.Helper()
	return decode[struct {
		ID int64 `json:"id"`
	}](t, e.send("GET", "/api/v1/me", session, nil)).ID
}
