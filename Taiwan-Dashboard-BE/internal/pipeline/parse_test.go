package pipeline

import (
	"testing"
	"time"
)

func TestParseNumber(t *testing.T) {
	cases := []struct {
		in   string
		want *float64
		err  bool
	}{
		{"695.1", ptr(695.1), false},
		{" 20,441.10 ", ptr(20441.1), false},
		{"93.591%", ptr(93.591), false},
		{"-259.3", ptr(-259.3), false},
		{"", nil, false},
		{"-", nil, false},
		{"ND", nil, false},
		{"abc", nil, true},
		{"NaN", nil, true},
	}
	for _, c := range cases {
		got, err := ParseNumber(c.in)
		if (err != nil) != c.err {
			t.Errorf("ParseNumber(%q) err = %v, want err %v", c.in, err, c.err)
			continue
		}
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("ParseNumber(%q) = %v, want %v", c.in, deref(got), deref(c.want))
		}
	}
}

func TestParseTaipeiTime(t *testing.T) {
	got, err := ParseTaipeiTime("2026-10-20T15:10:00", "2006/01/02 15:04:05", "2006-01-02T15:04:05")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 20, 7, 10, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got.UTC(), want)
	}
	if _, err := ParseTaipeiTime("yesterday", "2006-01-02"); err == nil {
		t.Error("expected an error for an unknown format")
	}
}

func TestNormalizeCountyText(t *testing.T) {
	cases := map[string]string{
		"台北市":          "臺北市",
		"基隆巿安樂區":       "基隆市安樂區",
		"桃園市龍潭區、\n大溪區": "桃園市龍潭區、大溪區",
	}
	for in, want := range cases {
		if got := NormalizeCountyText(in); got != want {
			t.Errorf("NormalizeCountyText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountiesPrefixPrefersLongerName(t *testing.T) {
	c := NewCounties(map[string][]string{
		"10018": {"新竹市", "新竹巿"},
		"10004": {"新竹縣"},
		"63000": {"臺北市", "台北"},
	})
	if code, _ := c.CodeFromAddress("新竹縣寶山鄉"); code != "10004" {
		t.Errorf("新竹縣寶山鄉 → %q, want 10004", code)
	}
	if code, _ := c.Code("台北市"); code != "63000" {
		t.Errorf("台北市 → %q, want 63000", code)
	}
	if _, ok := c.CodeFromAddress("火星"); ok {
		t.Error("expected no match")
	}
}

func TestDateColumn(t *testing.T) {
	got := DateColumn(time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 10, 20, 0, 0, 0, 0, Taipei); !got.Equal(want) {
		t.Errorf("DateColumn = %v, want %v", got, want)
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("一二三四五", 3); got != "一二三…" {
		t.Errorf("Truncate = %q", got)
	}
	if got := Truncate("abc", 5); got != "abc" {
		t.Errorf("Truncate = %q", got)
	}
}

func ptr(v float64) *float64 { return &v }

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
