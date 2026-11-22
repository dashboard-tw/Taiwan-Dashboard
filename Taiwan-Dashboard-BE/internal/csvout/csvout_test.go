package csvout

import (
	"strings"
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	out := string(Bytes([]string{"name", "value", "at"}, [][]any{
		{"=HYPERLINK(\"x\")", 12.5, time.Date(2026, 10, 22, 1, 0, 0, 0, time.UTC)},
		{"中山", nil, nil},
	}))
	want := BOM + "name,value,at\n" +
		`"'=HYPERLINK(""x"")",12.5,2026-10-22T09:00:00+08:00` + "\n" +
		"中山,,\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
	if !strings.HasPrefix(Value(1e21), "1000000000000000000000") {
		t.Errorf("large numbers must not use exponents: %s", Value(1e21))
	}
}
