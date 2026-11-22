package csvout

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
)

var taipei = time.FixedZone("Asia/Taipei", 8*60*60)

const BOM = "\xef\xbb\xbf"

type Writer struct {
	w *csv.Writer
}

func New(w io.Writer, columns []string) (*Writer, error) {
	if _, err := io.WriteString(w, BOM); err != nil {
		return nil, err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(columns); err != nil {
		return nil, err
	}
	return &Writer{w: cw}, nil
}

func (w *Writer) Row(values []any) error {
	rec := make([]string, len(values))
	for i, v := range values {
		rec[i] = Value(v)
	}
	return w.w.Write(rec)
}

func (w *Writer) Flush() error {
	w.w.Flush()
	return w.w.Error()
}

func Bytes(columns []string, rows [][]any) []byte {
	var buf bytes.Buffer
	w, _ := New(&buf, columns)
	for _, r := range rows {
		_ = w.Row(r)
	}
	_ = w.Flush()
	return buf.Bytes()
}

func Value(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case time.Time:
		return x.In(taipei).Format(time.RFC3339)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case []byte:
		return Text(string(x))
	case string:
		return Text(x)
	default:
		b, _ := json.Marshal(x)
		return strings.Trim(string(b), `"`)
	}
}

func Text(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
