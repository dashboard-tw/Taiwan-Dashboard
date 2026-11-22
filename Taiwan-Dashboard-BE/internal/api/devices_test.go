package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/auth"
)

func TestDevices(t *testing.T) {
	e := newMeEnv(t)
	ctx := context.Background()
	laptop := e.signInAs(t, auth.RoleUser, "active")
	me := decode[auth.User](t, e.send("GET", "/api/v1/me", laptop, nil))
	phone, _, err := e.store.CreateSession(ctx, me.ID, "203.0.113.9", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Safari/604.1")
	if err != nil {
		t.Fatal(err)
	}
	tablet, _, _ := e.store.CreateSession(ctx, me.ID, "203.0.113.10", "tablet")
	other := e.signInAs(t, auth.RoleUser, "active")

	for _, ep := range []struct{ method, path string }{
		{"GET", "/api/v1/me/devices"}, {"DELETE", "/api/v1/me/devices"}, {"DELETE", "/api/v1/me/devices/1"},
	} {
		if w := e.send(ep.method, ep.path, "", nil); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: %d", ep.method, ep.path, w.Code)
		}
	}

	list := decode[[]auth.Device](t, e.send("GET", "/api/v1/me/devices", laptop, nil))
	if len(list) != 3 {
		t.Fatalf("devices = %+v, want laptop, phone and tablet", list)
	}
	var phoneID int64
	currents := 0
	for _, d := range list {
		if d.Current {
			currents++
		}
		if d.IP == "203.0.113.9" {
			phoneID = d.ID
		}
		if d.LastSeenAt == nil {
			t.Errorf("device %d has no last-seen time", d.ID)
		}
	}
	if currents != 1 || phoneID == 0 || !list[0].Current {
		t.Fatalf("current=%d phone=%d in %+v", currents, phoneID, list)
	}
	if w := e.send("GET", "/api/v1/me/devices", laptop, nil); containsAny(w.Body.String(), laptop, phone, "id_hash") {
		t.Error("device list leaks a token")
	}

	if w := e.send("DELETE", fmt.Sprintf("/api/v1/me/devices/%d", phoneID), other, nil); w.Code != http.StatusNotFound {
		t.Errorf("another user signing out my phone: %d, want 404", w.Code)
	}
	if l := decode[[]auth.Device](t, e.send("GET", "/api/v1/me/devices", other, nil)); len(l) != 1 {
		t.Errorf("other user sees %d devices", len(l))
	}
	if w := e.send("GET", "/api/v1/me", phone, nil); w.Code != http.StatusOK {
		t.Fatalf("phone signed out by someone else: %d", w.Code)
	}

	w := e.send("DELETE", fmt.Sprintf("/api/v1/me/devices/%d", phoneID), laptop, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("sign out phone: %d %s", w.Code, w.Body)
	}
	for _, ck := range w.Result().Cookies() {
		if ck.MaxAge < 0 {
			t.Error("signing out another device cleared this browser's cookie")
		}
	}
	if w := e.send("GET", "/api/v1/me", phone, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("phone still signed in: %d", w.Code)
	}
	if w := e.send("DELETE", fmt.Sprintf("/api/v1/me/devices/%d", phoneID), laptop, nil); w.Code != http.StatusNotFound {
		t.Errorf("signing out twice: %d, want 404", w.Code)
	}
	for _, bad := range []string{"abc", "0", "-1"} {
		if w := e.send("DELETE", "/api/v1/me/devices/"+bad, laptop, nil); w.Code != http.StatusNotFound {
			t.Errorf("DELETE devices/%s: %d", bad, w.Code)
		}
	}

	res := decode[map[string]int](t, e.send("DELETE", "/api/v1/me/devices", laptop, nil))
	if res["signed_out"] != 1 {
		t.Errorf("signed_out = %v, want the tablet", res)
	}
	for tok, want := range map[string]int{tablet: 401, laptop: 200, other: 200} {
		if w := e.send("GET", "/api/v1/me", tok, nil); w.Code != want {
			t.Errorf("after sign-out-others: %d, want %d", w.Code, want)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action IN ('session.revoke', 'session.revoke_others') AND user_id = $1`, me.ID); n != 2 {
		t.Errorf("audit rows = %d, want 2", n)
	}

	list = decode[[]auth.Device](t, e.send("GET", "/api/v1/me/devices", laptop, nil))
	w = e.send("DELETE", fmt.Sprintf("/api/v1/me/devices/%d", list[0].ID), laptop, nil)
	cleared := false
	for _, ck := range w.Result().Cookies() {
		cleared = cleared || (ck.Name == "dtw_session" && ck.MaxAge < 0)
	}
	if w.Code != http.StatusNoContent || !cleared {
		t.Errorf("sign out this device: %d, cookie cleared %v", w.Code, cleared)
	}
}

func TestPurgePersonalData(t *testing.T) {
	e := newMeEnv(t)
	tok := e.signInAs(t, auth.RoleUser, "active")
	now := time.Now()
	if _, err := e.db.Exec(`
		INSERT INTO app.session (id_hash, user_id, created_at, expires_at, revoked_at)
		SELECT decode(repeat(h, 32), 'hex'), (SELECT user_id FROM app.session LIMIT 1), $1::timestamptz - interval '40 days', e, r
		  FROM (VALUES ('a1', $1::timestamptz - interval '31 days', NULL::timestamptz),
		               ('b2', $1::timestamptz + interval '1 day',  $1::timestamptz - interval '31 days'),
		               ('c3', $1::timestamptz + interval '1 day',  $1::timestamptz - interval '29 days')) v(h, e, r)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`
		INSERT INTO app.audit_log (action, ip, created_at) VALUES
		  ('old', '198.51.100.1', $1::timestamptz - interval '91 days'),
		  ('new', '198.51.100.2', $1::timestamptz - interval '89 days'),
		  ('ancient', NULL, $1::timestamptz - interval '366 days')`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`
		WITH k AS (
		  INSERT INTO app.api_key (user_id, name, prefix, key_hash, tier, revoked_at)
		  SELECT (SELECT user_id FROM app.session LIMIT 1), n, n, decode(repeat(h, 32), 'hex'), 'free', r
		    FROM (VALUES ('gone', 'd4', $1::timestamptz - interval '91 days'),
		                 ('recent', 'e5', $1::timestamptz - interval '89 days'),
		                 ('live', 'f6', NULL::timestamptz)) v(n, h, r)
		  RETURNING id, name)
		INSERT INTO app.api_usage_daily (api_key_id, day, requests) SELECT id, current_date, 1 FROM k`, now); err != nil {
		t.Fatal(err)
	}
	p, err := auth.Purge(context.Background(), e.store.DB, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Sessions != 2 || p.AuditIPs != 1 || p.Audit != 1 {
		t.Errorf("purged %+v, want 2 sessions, 1 IP and 1 audit entry", p)
	}
	if p.Clients != 1 {
		t.Errorf("purged %d deleted credentials, want the one deleted 91 days ago", p.Clients)
	}
	if n := e.count(t, `SELECT count(*) FROM app.api_key WHERE name IN ('recent', 'live')`); n != 2 {
		t.Errorf("credentials left = %d, want the recently deleted one and the live one", n)
	}
	if n := e.count(t, `SELECT count(*) FROM app.api_usage_daily`); n != 2 {
		t.Errorf("usage rows left = %d, want the two kept credentials' rows", n)
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'ancient'`); n != 0 {
		t.Error("an audit entry older than a year was kept")
	}
	if n := e.count(t, `SELECT count(*) FROM app.session`); n != 2 {
		t.Errorf("sessions left = %d, want the recent sign-out and the live one", n)
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action IN ('old', 'new') AND ip IS NULL`); n != 1 {
		t.Errorf("audit rows without IP = %d, want only the old one", n)
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'old'`); n != 1 {
		t.Error("the old audit entry itself was deleted; only its IP should go")
	}
	if w := e.send("GET", "/api/v1/me", tok, nil); w.Code != http.StatusOK {
		t.Errorf("live session purged: %d", w.Code)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if x != "" && strings.Contains(s, x) {
			return true
		}
	}
	return false
}
