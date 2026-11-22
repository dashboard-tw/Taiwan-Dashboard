package cli

import (
	"testing"

	"dashboard.tw/backend/internal/config"
	"dashboard.tw/backend/internal/sources/aviation"
	"dashboard.tw/backend/internal/sources/bike"
	"dashboard.tw/backend/internal/sources/ev"
	"dashboard.tw/backend/internal/sources/rail"
	"dashboard.tw/backend/internal/sources/tdx"
)

func tdxAccountOf(t *testing.T, cfg config.Config) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, s := range registry(cfg) {
		var c *tdx.Client
		switch p := s.pipeline.(type) {
		case *rail.StationPipeline:
			c = p.TDX
		case *rail.AlertPipeline:
			c = p.TDX
		case *rail.LivePipeline:
			c = p.TDX
		case *bike.StationPipeline:
			c = p.TDX
		case *bike.AvailabilityPipeline:
			c = p.TDX
		case *aviation.RefPipeline:
			c = p.TDX
		case *aviation.FlightPipeline:
			c = p.TDX
		case *ev.StationPipeline:
			c = p.TDX
		case *ev.LivePipeline:
			c = p.TDX
		default:
			continue
		}
		out[s.pipeline.Dataset().ID] = c.ClientID
	}
	return out
}

func TestTDXAccountsSplitThePipelines(t *testing.T) {
	one := config.Config{TdxClientID: "a", TdxClientSecret: "s"}
	two := one
	two.TdxClientID2, two.TdxClientSecret2 = "b", "s"
	three := two
	three.TdxClientID3, three.TdxClientSecret3 = "c", "s"
	partial := two
	partial.TdxClientID3 = "c"

	cases := []struct {
		name string
		cfg  config.Config
		want map[string]string
	}{
		{"one account", one, map[string]string{
			"rail_alert": "a", "tra_live": "a", "tra_station": "a", "bike_availability": "a", "bike_station": "a",
			"flight": "a", "airport": "a", "ev_live": "a", "ev_station": "a",
		}},
		{"two accounts", two, map[string]string{
			"rail_alert": "a", "tra_live": "a", "tra_station": "a", "bike_availability": "b", "bike_station": "b",
			"flight": "a", "airport": "a", "ev_live": "b", "ev_station": "b",
		}},
		{"three accounts", three, map[string]string{
			"rail_alert": "a", "tra_live": "a", "tra_station": "a", "bike_availability": "b", "bike_station": "b",
			"flight": "c", "airport": "c", "ev_live": "c", "ev_station": "c",
		}},
		{"a half-set third account is ignored", partial, map[string]string{
			"rail_alert": "a", "tra_live": "a", "tra_station": "a", "bike_availability": "b", "bike_station": "b",
			"flight": "a", "airport": "a", "ev_live": "b", "ev_station": "b",
		}},
	}
	for _, c := range cases {
		got := tdxAccountOf(t, c.cfg)
		if len(got) != len(c.want) {
			t.Errorf("%s: %d TDX pipelines, want %d: %v", c.name, len(got), len(c.want), got)
		}
		for id, want := range c.want {
			if got[id] != want {
				t.Errorf("%s: %s uses account %q, want %q", c.name, id, got[id], want)
			}
		}
	}
}

func TestTDXAccountsShareALimiterPerAccount(t *testing.T) {
	cfg := config.Config{TdxClientID: "a", TdxClientSecret: "s", TdxClientID2: "b", TdxClientSecret2: "s", TdxClientID3: "c", TdxClientSecret3: "s"}
	limiters := map[string]map[*tdx.Limiter]bool{}
	for _, s := range registry(cfg) {
		var c *tdx.Client
		switch p := s.pipeline.(type) {
		case *rail.AlertPipeline:
			c = p.TDX
		case *rail.LivePipeline:
			c = p.TDX
		case *bike.AvailabilityPipeline:
			c = p.TDX
		case *bike.StationPipeline:
			c = p.TDX
		case *aviation.FlightPipeline:
			c = p.TDX
		case *ev.LivePipeline:
			c = p.TDX
		default:
			continue
		}
		if limiters[c.ClientID] == nil {
			limiters[c.ClientID] = map[*tdx.Limiter]bool{}
		}
		limiters[c.ClientID][c.Limiter] = true
	}
	for id, ls := range limiters {
		if len(ls) != 1 {
			t.Errorf("account %s has %d limiters, want 1", id, len(ls))
		}
	}
	if len(limiters) != 3 {
		t.Errorf("%d accounts in use, want 3", len(limiters))
	}
}
