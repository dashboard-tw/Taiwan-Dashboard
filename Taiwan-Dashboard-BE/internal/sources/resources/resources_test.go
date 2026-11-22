package resources

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

func TestParseMedical(t *testing.T) {
	list, err := ParseMedical(read(t, "testdata/clinic.csv"), "clinic")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("clinics %+v", list)
	}
	f := list[0]
	if f.ID == "" || f.Name == "" || f.Kind == "" || f.County == "" || f.ContractEnd == nil {
		t.Errorf("first %+v", f)
	}
	if list[3].County != "10021" {
		t.Errorf("the Tainan clinic keeps the old code: %q", list[3].County)
	}
}

func TestParseLTC(t *testing.T) {
	list, err := ParseLTC(read(t, "testdata/ltc.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("facilities %+v", list)
	}
	if list[0].ID != "2B1500001" || list[0].ABC != "B" || list[0].Lat == nil || len(list[0].Services) == 0 {
		t.Errorf("first %+v", list[0])
	}
	if list[2].ID != "B21400074" || list[2].Lat != nil {
		t.Errorf("the facility without coordinates %+v", list[2])
	}
}

func TestParseShelters(t *testing.T) {
	list, err := ParseShelters(read(t, "testdata/shelter.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("shelters %+v", list)
	}
	if list[0].ID != "91685" || !list[0].Open || list[0].Lat != nil {
		t.Errorf("open shelter %+v", list[0])
	}
	if list[1].Open || list[1].Indoor == nil || list[1].Lat == nil {
		t.Errorf("closed shelter %+v", list[1])
	}
}

func TestParseShelterReports(t *testing.T) {
	list, warnings, err := ParseShelterReports(read(t, "testdata/report.json"))
	if err != nil || len(warnings) != 0 {
		t.Fatal(err, warnings)
	}
	if len(list) != 3 {
		t.Fatalf("reports %+v", list)
	}
	r := list[0]
	if r.ShelterID == "" || r.Refugees == nil || r.ReportedAt == nil || r.OpenedAt.IsZero() ||
		r.OpenedAt.Location().String() != pipeline.Taipei.String() {
		t.Errorf("first %+v", r)
	}
	if list[2].ShelterID[:6] != "adhoc:" {
		t.Errorf("a shelter without a code gets a name-based id: %q", list[2].ShelterID)
	}
	if got := parseEMICTime("2026/10/22 下午 06:11:48"); got == nil || got.Hour() != 18 {
		t.Errorf("PM time %v", got)
	}
	if got := parseEMICTime("2026/10/23 上午 11:00:48"); got == nil || got.Hour() != 11 {
		t.Errorf("AM time %v", got)
	}
}

func TestShelterReportMissingFile(t *testing.T) {
	srv := pipelinetest.NewServer(t, map[string]string{"/report.json": "testdata/report.json"})
	srv.Fail("/report.json", 404)
	p := &ShelterReportPipeline{URL: srv.URL + "/report.json"}
	res, err := p.Run(context.Background(), srv.Env(nil, time.Now()))
	if err != nil {
		t.Fatalf("a missing file must not fail: %v", err)
	}
	if len(res.Warnings) != 1 || res.RowsFetched != 0 {
		t.Errorf("result %+v", res)
	}
	srv.Fail("/report.json", 500)
	if _, err := p.Run(context.Background(), srv.Env(nil, time.Now())); err == nil {
		t.Error("other errors must still fail")
	}
}

func TestRuns(t *testing.T) {
	db := testdb.New(t)
	srv := pipelinetest.NewServer(t, map[string]string{
		"/center.csv":  "testdata/center.csv",
		"/clinic.csv":  "testdata/clinic.csv",
		"/ltc.csv":     "testdata/ltc.csv",
		"/shelter.xml": "testdata/shelter.xml",
		"/report.json": "testdata/report.json",
	})
	ctx := context.Background()
	env := srv.Env(db, time.Date(2026, 10, 23, 17, 0, 0, 0, pipeline.Taipei))

	urls := map[string]string{}
	for _, f := range MedicalFiles {
		urls[f.Level] = srv.URL + "/center.csv"
	}
	urls["clinic"] = srv.URL + "/clinic.csv"
	med := &MedicalPipeline{URLs: urls}
	if res, err := med.Run(ctx, env); err != nil || len(res.Warnings) != 0 {
		t.Fatalf("medical %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.medical_facility WHERE county_code IS NULL`); n != 0 {
		t.Errorf("facilities without a county = %d", n)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.medical_facility WHERE county_code = '67000'`); n != 1 {
		t.Errorf("Tainan (old code 10021) = %d", n)
	}
	srv.Fail("/clinic.csv", 500)
	if _, err := med.Run(ctx, env); err == nil {
		t.Error("a failed file must fail the run")
	}

	if res, err := (&LTCPipeline{URL: srv.URL + "/ltc.csv"}).Run(ctx, env); err != nil || res.RowsFetched != 3 {
		t.Fatalf("ltc %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.ltc_facility WHERE geom IS NOT NULL`); n != 2 {
		t.Errorf("ltc with points = %d", n)
	}

	if res, err := (&ShelterPipeline{URL: srv.URL + "/shelter.xml"}).Run(ctx, env); err != nil || res.RowsFetched != 3 {
		t.Fatalf("shelters %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM ref.shelter WHERE is_open`); n != 1 {
		t.Errorf("open shelters = %d", n)
	}
	rep := &ShelterReportPipeline{URL: srv.URL + "/report.json"}
	if res, err := rep.Run(ctx, env); err != nil || res.RowsInserted != 3 {
		t.Fatalf("reports %+v, %v", res, err)
	}
	if res, err := rep.Run(ctx, env); err != nil || res.RowsInserted != 3 {
		t.Fatalf("second report run %+v, %v", res, err)
	}
	if n := pipelinetest.Count(t, db, `SELECT count(*) FROM data.shelter_report`); n != 3 {
		t.Errorf("report rows = %d", n)
	}
}
