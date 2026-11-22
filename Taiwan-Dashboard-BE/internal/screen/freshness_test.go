package screen

import (
	"testing"
	"time"

	"dashboard.tw/backend/internal/component"
)

func TestPausedAsOf(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	str := func(s string) *string { return &s }
	src := func(freq int64, staleAfter int64, ingested string) component.Source {
		return component.Source{Status: "error", UpdateFreqSeconds: freq, StaleAfterSeconds: staleAfter, LastIngestedAt: str(ingested)}
	}
	const h = 3600

	cases := []struct {
		name    string
		sources []component.Source
		want    string // "" means not paused
	}{
		{"recent fetch despite an error", []component.Source{src(600, h, "2026-11-20T11:50:00+08:00")}, ""},
		{"late but under six hours", []component.Source{src(600, h, "2026-11-20T07:00:00+08:00")}, ""},
		{"stopped for days", []component.Source{src(600, h, "2026-11-03T01:58:00+08:00")}, "2026-11-03T01:58:00+08:00"},
		{"slow dataset within stale_after", []component.Source{src(86400, 72*h, "2026-11-18T00:00:00+08:00")}, ""},
		{
			"live source with a stopped master file",
			[]component.Source{src(600, h, "2026-11-20T11:55:00+08:00"), src(86400, 168*h, "2026-10-01T00:00:00+08:00")},
			"",
		},
		{
			"stopped live source with a fresh master file",
			[]component.Source{src(600, h, "2026-11-03T02:03:00+08:00"), src(86400, 168*h, "2026-11-20T03:00:00+08:00")},
			"2026-11-03T02:03:00+08:00",
		},
		{
			"one of two same-pace sources still live",
			[]component.Source{src(600, h, "2026-11-17T11:37:00+08:00"), src(600, h, "2026-11-20T11:55:00+08:00")},
			"",
		},
		{
			"both same-pace sources stopped: the earlier fetch",
			[]component.Source{src(600, h, "2026-11-03T02:03:00+08:00"), src(600, h, "2026-11-03T01:58:00+08:00")},
			"2026-11-03T01:58:00+08:00",
		},
		{"pending", []component.Source{{Status: "pending", UpdateFreqSeconds: 600, StaleAfterSeconds: h}}, ""},
	}
	for _, c := range cases {
		got, ok := pausedAsOf(c.sources, now)
		switch {
		case c.want == "" && ok:
			t.Errorf("%s: paused as of %v, want not paused", c.name, got)
		case c.want != "" && (!ok || got.Format(time.RFC3339) != c.want):
			t.Errorf("%s: got %v %v, want %s", c.name, got, ok, c.want)
		}
	}
}
