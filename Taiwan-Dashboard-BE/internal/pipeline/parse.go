package pipeline

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

func StripBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
}

func ParseNumber(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.ReplaceAll(s, ",", "")
	switch strings.ToUpper(s) {
	case "", "-", "--", "ND", "NA", "N/A", "X", "NULL":
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("not a number: %q", s)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("not a finite number: %q", s)
	}
	return &v, nil
}

func ParseTaipeiTime(s string, layouts ...string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, Taipei); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised time %q", s)
}

func TaipeiDate(t time.Time) time.Time {
	y, m, d := t.In(Taipei).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, Taipei)
}

func DateColumn(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, Taipei)
}

func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
