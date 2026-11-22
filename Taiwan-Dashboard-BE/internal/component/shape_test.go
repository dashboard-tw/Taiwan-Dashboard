package component

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBuildShapes(t *testing.T) {
	ts := time.Date(2026, 10, 21, 7, 0, 0, 0, time.UTC)
	cases := []struct {
		shape string
		rows  Rows
		want  string
	}{
		{ShapeByCounty, Rows{Columns: []string{"county_code", "data"}, Values: [][]any{{"63000", "42.5"}, {"64000", nil}}},
			`{"data":[{"county_code":"63000","data":42.5},{"county_code":"64000","data":null}]}`},
		{ShapeTwoD, Rows{Columns: []string{"x_axis", "data"}, Values: [][]any{{"大同", int64(52)}}},
			`{"data":[{"data":[{"x":"大同","y":52}]}]}`},
		{ShapeTime, Rows{Columns: []string{"x_axis", "y_axis", "data"}, Values: [][]any{{ts, "AQI", 40.5}, {ts.Add(time.Hour), "AQI", 41.0}}},
			`{"data":[{"name":"AQI","data":[{"x":"2026-10-21T15:00:00+08:00","y":40.5},{"x":"2026-10-21T16:00:00+08:00","y":41}]}]}`},
		{ShapeMapLegend, Rows{Columns: []string{"name", "type", "value"}, Values: [][]any{{"測站", "circle", 3.0}}},
			`{"data":[{"name":"測站","type":"circle","value":3}]}`},
	}
	for _, c := range cases {
		got, err := Build(c.shape, c.rows)
		if err != nil {
			t.Errorf("%s: %v", c.shape, err)
			continue
		}
		if s := asJSON(t, got); s != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.shape, s, c.want)
		}
	}
}

func TestBuildThreeDAlignsByCategory(t *testing.T) {
	rows := Rows{Columns: []string{"x_axis", "y_axis", "data"}, Values: [][]any{
		{"臺北", "PM2.5", 10.0},
		{"臺中", "PM2.5", 20.0},
		{"臺中", "O3", 30.0}, // no 臺北 O3 row
	}}
	got, err := Build(ShapeThreeD, rows)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"data":[{"name":"PM2.5","data":[10,20]},{"name":"O3","data":[null,30]}],"categories":["臺北","臺中"]}`
	if s := asJSON(t, got); s != want {
		t.Errorf("got %s\nwant %s", s, want)
	}
}

func TestBuildRejectsWrongColumns(t *testing.T) {
	_, err := Build(ShapeByCounty, Rows{Columns: []string{"county", "value"}})
	if !errors.Is(err, ErrShape) {
		t.Errorf("missing columns: err = %v, want ErrShape", err)
	}
	_, err = Build(ShapeTwoD, Rows{Columns: []string{"x_axis", "data"}, Values: [][]any{{"a", "not a number"}}})
	if !errors.Is(err, ErrShape) {
		t.Errorf("non-numeric data: err = %v, want ErrShape", err)
	}
	_, err = Build(ShapeTime, Rows{Columns: []string{"x_axis", "y_axis", "data"}, Values: [][]any{{"2026-10-21", "a", 1.0}}})
	if !errors.Is(err, ErrShape) {
		t.Errorf("text x_axis for time: err = %v, want ErrShape", err)
	}
}
