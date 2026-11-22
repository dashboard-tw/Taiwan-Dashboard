package component

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const GeometryColumn = "geometry"

func BuildGeoJSON(rows Rows) ([]byte, error) {
	geomIdx := -1
	for i, c := range rows.Columns {
		if c == GeometryColumn {
			geomIdx = i
		}
	}
	if geomIdx < 0 {
		return nil, fmt.Errorf("%w: geojson_sql must return a %q column (ST_AsGeoJSON)", ErrShape, GeometryColumn)
	}

	var buf bytes.Buffer
	buf.WriteString(`{"type":"FeatureCollection","features":[`)
	first := true
	for _, r := range rows.Values {
		geom := text(r[geomIdx])
		if geom == "" {
			continue
		}
		if !json.Valid([]byte(geom)) {
			return nil, fmt.Errorf("%w: geometry is not valid GeoJSON", ErrShape)
		}
		props := map[string]any{}
		for i, c := range rows.Columns {
			if i != geomIdx {
				props[c] = property(r[i], columnType(rows, i))
			}
		}
		p, err := json.Marshal(props)
		if err != nil {
			return nil, err
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.WriteString(`{"type":"Feature","geometry":`)
		buf.WriteString(geom)
		buf.WriteString(`,"properties":`)
		buf.Write(p)
		buf.WriteByte('}')
	}
	buf.WriteString(`]}`)
	return buf.Bytes(), nil
}

func columnType(rows Rows, i int) string {
	if i < len(rows.Types) {
		return rows.Types[i]
	}
	return ""
}

func property(v any, typ string) any {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		return x.In(pipeline.Taipei).Format(time.RFC3339)
	case []byte, string:
		if typ == "NUMERIC" {
			if f, err := number(x); err == nil && f != nil {
				return *f
			}
		}
		return text(x)
	case float64, float32, int64, int32, int16, int, bool:
		return x
	default:
		return fmt.Sprint(x)
	}
}
