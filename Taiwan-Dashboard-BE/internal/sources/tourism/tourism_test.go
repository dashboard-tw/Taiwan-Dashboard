package tourism

import (
	"context"
	"os"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseAttractions(t *testing.T) {
	list, err := ParseAttractions(read(t, "testdata/attraction.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("attractions %+v", list)
	}
	a := list[0]
	if a.County != "10002" || a.Lat == nil || len(a.Classes) == 0 || a.Address == "" || a.Status == nil {
		t.Errorf("first %+v", a)
	}
	if list[3].Free == nil || !*list[3].Free {
		t.Errorf("the free attraction %+v", list[3])
	}
}

func TestParseHotels(t *testing.T) {
	list, err := ParseHotels(read(t, "testdata/hotel.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("hotels %+v", list)
	}
	h := list[0]
	if h.Class == nil || *h.Class != 4 || h.Rooms == nil || h.LowestPrice == nil || h.Lat == nil || h.Stars != nil {
		t.Errorf("first %+v", h)
	}
	if list[2].Class == nil || *list[2].Class != 1 {
		t.Errorf("the international hotel %+v", list[2])
	}
	if _, err := ParseHotels([]byte("not a zip")); err == nil {
		t.Error("a broken zip must fail")
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/attraction.zip": "testdata/attraction.zip",
		"/hotel.zip":      "testdata/hotel.zip",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))
	if res, err := (&AttractionPipeline{URL: srv.URL + "/attraction.zip"}).Run(ctx, env); err != nil ||
		res.RowsInserted != 4 || len(res.Warnings) != 0 {
		t.Fatalf("attractions %+v, %v", res, err)
	}
	if res, err := (&HotelPipeline{URL: srv.URL + "/hotel.zip"}).Run(ctx, env); err != nil ||
		res.RowsInserted != 3 || len(res.Warnings) != 0 {
		t.Fatalf("hotels %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.tourism_attraction WHERE geom IS NOT NULL AND cardinality(classes) > 0`); n != 4 {
		t.Errorf("attractions with points and classes = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.tourism_hotel WHERE county_code IS NOT NULL AND geom IS NOT NULL`); n != 3 {
		t.Errorf("located hotels = %d", n)
	}
}
