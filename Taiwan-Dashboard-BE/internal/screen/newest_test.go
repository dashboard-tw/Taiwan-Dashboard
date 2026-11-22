package screen

import (
	"testing"

	"dashboard.tw/backend/internal/component"
)

func TestNewestUsesTheFastestSource(t *testing.T) {
	at := func(s string) *string { return &s }
	sources := []component.Source{
		{Dataset: "river_level", LastObservedAt: at("2026-10-22T10:20:00+08:00"), UpdateFreqSeconds: 600},
		{Dataset: "river_station", LastObservedAt: at("2026-10-22T10:41:03+08:00"), UpdateFreqSeconds: 86400},
	}
	if got := newest(sources); got == nil || *got != "2026-10-22T10:20:00+08:00" {
		t.Errorf("newest = %v", got)
	}
	sources[0].LastObservedAt = nil
	if got := newest(sources); got == nil || *got != "2026-10-22T10:41:03+08:00" {
		t.Errorf("newest without readings = %v", got)
	}
	if newest(nil) != nil {
		t.Error("no sources should give no time")
	}
}
