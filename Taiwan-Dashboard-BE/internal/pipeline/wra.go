package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func DecodeWRAList(body []byte) ([]map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var raw []map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode WRA response: %w", err)
	}
	out := make([]map[string]string, len(raw))
	for i, m := range raw {
		rec := make(map[string]string, len(m))
		for k, v := range m {
			switch x := v.(type) {
			case string:
				rec[k] = x
			case json.Number:
				rec[k] = x.String()
			case nil:
				rec[k] = ""
			default:
				rec[k] = fmt.Sprint(x)
			}
		}
		out[i] = rec
	}
	return out, nil
}
