package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dashboard.tw/backend/internal/testdb"
)

type plainSealer struct{}

func (plainSealer) Seal(_ int64, s string) ([]byte, error) { return []byte(s), nil }
func (plainSealer) Open(_ int64, b []byte) (string, error) { return string(b), nil }

func TestCheck(t *testing.T) {
	for _, c := range []struct {
		kind string
		t    Target
		ok   bool
	}{
		{"discord", Target{URL: "https://discord.com/api/webhooks/123/abcdef"}, true},
		{"discord", Target{URL: "https://evil.example/api/webhooks/1/2"}, false},
		{"discord", Target{URL: "http://discord.com/api/webhooks/1/2"}, false},
		{"slack", Target{URL: "https://hooks.slack.com/services/T/B/x"}, true},
		{"slack", Target{URL: "https://hooks.slack.com/other"}, false},
		{"telegram", Target{Token: "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", Chat: "-1001234567"}, true},
		{"telegram", Target{Token: "nope", Chat: "-1001234567"}, false},
		{"json", Target{URL: "https://example.com/hook"}, true},
		{"json", Target{URL: "https://example.com/hook", Secret: "short"}, false},
		{"email", Target{URL: "https://example.com"}, false},
	} {
		hint, err := Check(c.kind, c.t)
		if (err == nil) != c.ok {
			t.Errorf("Check(%s, %+v) = %q, %v", c.kind, c.t, hint, err)
		}
		if err == nil && (strings.Contains(hint, "abcdef") || strings.Contains(hint, "ABCDEFGH")) {
			t.Errorf("hint %q shows the secret part", hint)
		}
	}
}

func TestSafeClientBlocksLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, err := SafeClient().Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "blocked address") {
		t.Errorf("loopback was reachable: %v", err)
	}
}

func TestSendFormats(t *testing.T) {
	var mu sync.Mutex
	got := map[string]map[string]any{}
	headers := map[string]http.Header{}
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		mu.Lock()
		got[r.URL.Path], headers[r.URL.Path] = v, r.Header
		mu.Unlock()
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "3")
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	m := Message{Title: "新應用上架：<雨天>", Text: "說明", URL: "https://dashboard.tw/showcase/app",
		Level: LevelOK, Fields: []Field{{"作者", "小明"}}}
	ctx := context.Background()
	c := srv.Client()
	for _, kind := range []string{"discord", "slack", "json"} {
		tg := Target{URL: srv.URL + "/" + kind}
		if kind == "json" {
			tg.Secret = strings.Repeat("s", 16)
		}
		if err := Send(ctx, c, kind, tg, EventAppPublished, m); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	embed := got["/discord"]["embeds"].([]any)[0].(map[string]any)
	if embed["title"] != m.Title || embed["color"].(float64) != float64(colors[LevelOK]) {
		t.Errorf("discord embed = %v", embed)
	}
	if mentions := got["/discord"]["allowed_mentions"].(map[string]any)["parse"].([]any); len(mentions) != 0 {
		t.Error("discord mentions are not disabled")
	}
	blocks := fmt.Sprint(got["/slack"]["blocks"])
	if !strings.Contains(blocks, "&lt;雨天&gt;") {
		t.Errorf("slack text not escaped: %s", blocks)
	}
	if got["/json"]["event"] != EventAppPublished || !strings.HasPrefix(headers["/json"].Get("X-Dashboard-Signature"), "sha256=") {
		t.Errorf("json = %v %v", got["/json"], headers["/json"])
	}
	if !strings.Contains(telegramText(m), "<b>新應用上架：&lt;雨天&gt;</b>") {
		t.Errorf("telegram = %q", telegramText(m))
	}

	status = http.StatusTooManyRequests
	err := Send(ctx, c, "discord", Target{URL: srv.URL + "/discord"}, EventAppPublished, m)
	var retry *RetryError
	if !errors.As(err, &retry) || retry.After != 3*time.Second {
		t.Errorf("429: %v", err)
	}
	status = http.StatusNotFound
	err = Send(ctx, c, "discord", Target{URL: srv.URL + "/discord"}, EventAppPublished, m)
	var perm *PermanentError
	if !errors.As(err, &perm) {
		t.Errorf("404 should not be retried: %v", err)
	}
}

func TestEmitAndDispatch(t *testing.T) {
	db := testdb.App(t, testdb.New(t))
	ctx := context.Background()
	var mu sync.Mutex
	var titles []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		_ = json.NewDecoder(r.Body).Decode(&v)
		mu.Lock()
		titles = append(titles, r.URL.Path+":"+v["title"].(string))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	add := func(name, path string, events []string, enabled bool) {
		enc, _ := SealTarget(plainSealer{}, Target{URL: srv.URL + path})
		if _, err := db.Exec(`INSERT INTO app.notify_channel (name, kind, target_enc, events, enabled) VALUES ($1, 'json', $2, $3, $4)`,
			name, enc, events, enabled); err != nil {
			t.Fatal(err)
		}
	}
	add("public", "/public", []string{EventAppPublished}, true)
	add("admins", "/admins", []string{EventAppSubmitted, EventPipelineFailed}, true)
	add("off", "/off", []string{EventAppPublished}, false)

	d := NewDispatcher(db, plainSealer{})
	d.Client = srv.Client()
	d.Emit(ctx, EventAppPublished, Message{Title: "上架", Level: LevelOK})
	d.Emit(ctx, EventAppSubmitted, Message{Title: "投稿", Level: LevelInfo})
	d.Emit(ctx, EventMailFailed, Message{Title: "沒人訂", Level: LevelError})
	for range 10 {
		worked, err := d.sendNext(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	if strings.Join(titles, ",") != "/public:上架,/admins:投稿" {
		t.Errorf("sent = %v", titles)
	}

	titles = nil
	if _, err := db.Exec(`INSERT INTO app.dataset (id, name, source_agency, source_url, license, update_freq, stale_after)
		VALUES ('test_ds', '測試資料', 'x', 'https://example.com', 'x', '10 minutes', '1 hour') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	run := func(status string) {
		if _, err := db.Exec(`INSERT INTO app.pipeline_run (dataset_id, status, finished_at) VALUES ('test_ds', $1, now())`, status); err != nil {
			t.Fatal(err)
		}
		var runErr error
		if status == "failed" {
			runErr = errors.New("upstream 500")
		}
		PipelineFinished(ctx, db, "https://dashboard.tw", "test_ds", "測試資料", runErr)
	}
	for _, s := range []string{"failed", "failed", "success", "failed", "failed", "failed", "failed", "success", "success"} {
		run(s)
	}
	for range 10 {
		if worked, err := d.sendNext(ctx); err != nil || !worked {
			break
		}
	}
	if strings.Join(titles, ",") != "/admins:資料管線連續失敗：測試資料,/admins:資料管線恢復正常：測試資料" {
		t.Errorf("pipeline alerts = %v", titles)
	}
}

func TestDiscordButtonsAndFallback(t *testing.T) {
	var bodies []map[string]any
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		_ = json.NewDecoder(r.Body).Decode(&v)
		bodies = append(bodies, v)
		queries = append(queries, r.URL.RawQuery)
		if _, ok := v["components"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	m := Message{Title: "t", Level: LevelOK, Image: "https://x/i.png", Thumbnail: "https://x/l.png",
		Links: []Link{{Label: "前往網站", URL: "https://a.example", Icon: "web"}}}
	if err := Send(context.Background(), srv.Client(), "discord", Target{URL: srv.URL + "/w"}, EventAppPublished, m); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || queries[0] != "with_components=true" {
		t.Fatalf("calls = %d queries = %v", len(bodies), queries)
	}
	embed := bodies[0]["embeds"].([]any)[0].(map[string]any)
	if embed["image"].(map[string]any)["url"] != "https://x/i.png" || embed["thumbnail"].(map[string]any)["url"] != "https://x/l.png" {
		t.Errorf("embed images = %v", embed)
	}
	desc := bodies[1]["embeds"].([]any)[0].(map[string]any)["description"].(string)
	if !strings.Contains(desc, "前往網站：https://a.example") {
		t.Errorf("fallback text = %q", desc)
	}
	if tg, _ := telegramRequest(Target{Token: "1:x", Chat: "1"}, m); !strings.HasSuffix(tg, "/sendPhoto") {
		t.Errorf("telegram with image should use sendPhoto: %s", tg)
	}
}

func TestParseEmojisAndButtonIcon(t *testing.T) {
	m, err := ParseEmojis("web:dtw_web:111, site:dtw_site:222")
	if err != nil || m["web"].ID != "111" || m["site"].Name != "dtw_site" {
		t.Fatalf("parse = %v, %v", m, err)
	}
	for _, bad := range []string{"web:dtw_web", "web:x:abc"} {
		if _, err := ParseEmojis(bad); err == nil {
			t.Errorf("ParseEmojis(%q) accepted", bad)
		}
	}
	old := DiscordEmojis
	DiscordEmojis = m
	defer func() { DiscordEmojis = old }()
	body := discordBody(Message{Title: "t", Links: []Link{{Label: "前往網站", URL: "https://a", Icon: "web"}, {Label: "x", URL: "https://b", Icon: "none"}}})
	btns := body["components"].([]any)[0].(map[string]any)["components"].([]map[string]any)
	if e, ok := btns[0]["emoji"].(map[string]string); !ok || e["id"] != "111" {
		t.Errorf("web button emoji = %v", btns[0]["emoji"])
	}
	if _, ok := btns[1]["emoji"]; ok {
		t.Error("unknown icon got an emoji")
	}
}
