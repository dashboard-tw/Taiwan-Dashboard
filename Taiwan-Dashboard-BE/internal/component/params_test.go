package component

import (
	"strings"
	"testing"
	"time"
)

func TestCompile(t *testing.T) {
	cases := []struct {
		in, want string
		names    []string
	}{
		{"SELECT 1", "SELECT 1", nil},
		{"WHERE c = :county", "WHERE c = $1", []string{"county"}},
		{
			"WHERE (:county::char(5) IS NULL OR c = :county) AND t >= :from AND t < :to",
			"WHERE ($1::char(5) IS NULL OR c = $1) AND t >= $2 AND t < $3",
			[]string{"county", "from", "to"},
		},
		{"SELECT date_trunc(:step, t)", "SELECT date_trunc($1, t)", []string{"step"}},
		{"SELECT x::text, ':county' AS s", "SELECT x::text, ':county' AS s", nil},
		{`SELECT "a:county", 'it''s :to'`, `SELECT "a:county", 'it''s :to'`, nil},
		{"SELECT 1 -- :from\n, :to", "SELECT 1 -- :from\n, $1", []string{"to"}},
		{"SELECT /* :from */ :to", "SELECT /* :from */ $1", []string{"to"}},
		{"SELECT $q$ :from $q$, :to", "SELECT $q$ :from $q$, $1", []string{"to"}},
		{"SELECT arr[1:2]", "SELECT arr[1:2]", nil},
		{"WHERE t >= now() - interval '1 hour'", "WHERE t >= (" + AsOfSQL + ") - interval '1 hour'", nil},
		{"WHERE t > NOW ( ) AND d = current_date", "WHERE t > (" + AsOfSQL + ") AND d = ((" + AsOfSQL + ")::date)", nil},
		{"SELECT CURRENT_TIMESTAMP, :to", "SELECT (" + AsOfSQL + "), $1", []string{"to"}},
		{"SELECT 'now()', x.now, snow(), now_ms, a AS now", "SELECT 'now()', x.now, snow(), now_ms, a AS now", nil},
		{"SELECT 1 -- now()\n", "SELECT 1 -- now()\n", nil},
		{"SELECT (now() AT TIME ZONE 'Asia/Taipei')::date", "SELECT ((" + AsOfSQL + ") AT TIME ZONE 'Asia/Taipei')::date", nil},
	}
	for _, c := range cases {
		got, err := Compile(c.in)
		if err != nil {
			t.Errorf("Compile(%q): %v", c.in, err)
			continue
		}
		if got.SQL != c.want || strings.Join(got.names, ",") != strings.Join(c.names, ",") {
			t.Errorf("Compile(%q) = %q %v, want %q %v", c.in, got.SQL, got.names, c.want, c.names)
		}
	}
}

func TestCompileRejects(t *testing.T) {
	for _, in := range []string{
		"WHERE id = :user_id", // only the four documented parameters
		"WHERE id = $1",       // no positional placeholders
		"SELECT /* unterminated",
		"SELECT $x$ unterminated",
	} {
		if _, err := Compile(in); err == nil {
			t.Errorf("Compile(%q) should fail", in)
		}
	}
}

func TestArgs(t *testing.T) {
	q, _ := Compile("WHERE c = :county AND t BETWEEN :from AND :to AND s = :step AND c2 = :county")
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	county := "63000"

	args := q.Args(Params{County: &county, From: from, To: to, Step: "hour"})
	if len(args) != 4 || args[0] != "63000" || args[1] != from || args[2] != to || args[3] != "hour" {
		t.Errorf("args = %v", args)
	}
	if args := q.Args(Params{From: from, To: to}); args[0] != nil {
		t.Errorf("national county arg = %v, want nil (SQL NULL)", args[0])
	}
	ty, _ := Compile("WHERE typhoon_id = :typhoon")
	id := "2026-33"
	if args := ty.Args(Params{Typhoon: &id}); len(args) != 1 || args[0] != "2026-33" {
		t.Errorf("typhoon args = %v", args)
	}
	if args := ty.Args(Params{}); args[0] != nil {
		t.Errorf("no typhoon = %v, want nil", args[0])
	}
}

func TestStepFor(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for _, c := range []struct {
		span time.Duration
		want string
	}{
		{day, "hour"}, {2 * day, "day"}, {31 * day, "day"}, {60 * day, "week"},
		{365 * day, "month"}, {731 * day, "month"}, {1000 * day, "year"},
	} {
		if got := StepFor(base, base.Add(c.span)); got != c.want {
			t.Errorf("StepFor(%v) = %s, want %s", c.span, got, c.want)
		}
	}
}
