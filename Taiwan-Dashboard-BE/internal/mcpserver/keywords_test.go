package mcpserver

import (
	"slices"
	"testing"
)

func TestExpandQuery(t *testing.T) {
	got := expandQuery("Air Quality in Taipei")
	for _, want := range []string{"空氣品質", "空品", "taipei"} {
		if !slices.Contains(got, want) {
			t.Errorf("expandQuery = %v, missing %q", got, want)
		}
	}
	if got := expandQuery("power outage"); !slices.Contains(got, "停電") {
		t.Errorf("power outage = %v", got)
	}
	if got := expandQuery("台北 水庫"); !slices.Contains(got, "臺北") || !slices.Contains(got, "水庫") {
		t.Errorf("chinese = %v", got)
	}
}
