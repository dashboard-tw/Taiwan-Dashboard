package component

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	ShapeByCounty  = "by_county"
	ShapeTwoD      = "two_d"
	ShapeThreeD    = "three_d"
	ShapePercent   = "percent"
	ShapeTime      = "time"
	ShapeMapLegend = "map_legend"
)

var requiredColumns = map[string][]string{
	ShapeByCounty:  {"county_code", "data"},
	ShapeTwoD:      {"x_axis", "data"},
	ShapeThreeD:    {"x_axis", "y_axis", "data"},
	ShapePercent:   {"x_axis", "y_axis", "data"},
	ShapeTime:      {"x_axis", "y_axis", "data"},
	ShapeMapLegend: {"name", "type"},
}

func ValidShape(s string) bool {
	_, ok := requiredColumns[s]
	return ok
}

var ErrShape = errors.New("query result does not match its data shape")

type Rows struct {
	Columns []string
	Values  [][]any
	Types   []string
}

type Chart struct {
	Data       any      `json:"data"`
	Categories []string `json:"categories,omitempty"`
}

func CheckColumns(shape string, columns []string) error {
	need, ok := requiredColumns[shape]
	if !ok {
		return fmt.Errorf("%w: unknown data shape %q", ErrShape, shape)
	}
	have := map[string]bool{}
	for _, c := range columns {
		have[c] = true
	}
	var missing []string
	for _, c := range need {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s needs columns %s, missing %s",
			ErrShape, shape, strings.Join(need, ", "), strings.Join(missing, ", "))
	}
	return nil
}

func Build(shape string, rows Rows) (Chart, error) {
	if err := CheckColumns(shape, rows.Columns); err != nil {
		return Chart{}, err
	}
	col := map[string]int{}
	for i, c := range rows.Columns {
		col[c] = i
	}
	get := func(r []any, name string) any { return r[col[name]] }

	switch shape {
	case ShapeByCounty:
		out := make([]byCounty, 0, len(rows.Values))
		for _, r := range rows.Values {
			v, err := number(get(r, "data"))
			if err != nil {
				return Chart{}, err
			}
			out = append(out, byCounty{County: text(get(r, "county_code")), Data: v})
		}
		return Chart{Data: out}, nil

	case ShapeTwoD:
		points := make([]point, 0, len(rows.Values))
		for _, r := range rows.Values {
			v, err := number(get(r, "data"))
			if err != nil {
				return Chart{}, err
			}
			points = append(points, point{X: text(get(r, "x_axis")), Y: v})
		}
		return Chart{Data: []series{{Data: points}}}, nil

	case ShapeThreeD, ShapePercent:
		var categories []string
		catIndex := map[string]int{}
		var names []string
		values := map[string]map[int]*float64{}
		for _, r := range rows.Values {
			x, y := text(get(r, "x_axis")), text(get(r, "y_axis"))
			v, err := number(get(r, "data"))
			if err != nil {
				return Chart{}, err
			}
			if _, ok := catIndex[x]; !ok {
				catIndex[x] = len(categories)
				categories = append(categories, x)
			}
			if _, ok := values[y]; !ok {
				names = append(names, y)
				values[y] = map[int]*float64{}
			}
			values[y][catIndex[x]] = v
		}
		out := make([]namedValues, 0, len(names))
		for _, n := range names {
			vs := make([]*float64, len(categories))
			for i := range categories {
				vs[i] = values[n][i]
			}
			out = append(out, namedValues{Name: n, Data: vs})
		}
		return Chart{Data: out, Categories: categories}, nil

	case ShapeTime:
		var names []string
		byName := map[string][]timePoint{}
		for _, r := range rows.Values {
			t, ok := get(r, "x_axis").(time.Time)
			if !ok {
				return Chart{}, fmt.Errorf("%w: time needs x_axis to be a timestamp, got %T", ErrShape, get(r, "x_axis"))
			}
			v, err := number(get(r, "data"))
			if err != nil {
				return Chart{}, err
			}
			y := text(get(r, "y_axis"))
			if _, ok := byName[y]; !ok {
				names = append(names, y)
			}
			byName[y] = append(byName[y], timePoint{X: t.In(pipeline.Taipei).Format(time.RFC3339), Y: v})
		}
		out := make([]namedPoints, 0, len(names))
		for _, n := range names {
			out = append(out, namedPoints{Name: n, Data: byName[n]})
		}
		return Chart{Data: out}, nil

	case ShapeMapLegend:
		out := make([]map[string]any, 0, len(rows.Values))
		for _, r := range rows.Values {
			item := map[string]any{"name": text(get(r, "name")), "type": text(get(r, "type"))}
			for _, extra := range []string{"icon", "value"} {
				if i, ok := col[extra]; ok {
					item[extra] = r[i]
				}
			}
			out = append(out, item)
		}
		return Chart{Data: out}, nil
	}
	return Chart{}, fmt.Errorf("%w: unknown data shape %q", ErrShape, shape)
}

type byCounty struct {
	County string   `json:"county_code"`
	Data   *float64 `json:"data"`
}

type point struct {
	X string   `json:"x"`
	Y *float64 `json:"y"`
}

type series struct {
	Data []point `json:"data"`
}

type namedValues struct {
	Name string     `json:"name"`
	Data []*float64 `json:"data"`
}

type timePoint struct {
	X string   `json:"x"`
	Y *float64 `json:"y"`
}

type namedPoints struct {
	Name string      `json:"name"`
	Data []timePoint `json:"data"`
}

func number(v any) (*float64, error) {
	var f float64
	switch x := v.(type) {
	case nil:
		return nil, nil
	case float64:
		f = x
	case float32:
		f = float64(x)
	case int64:
		f = float64(x)
	case int32:
		f = float64(x)
	case int16:
		f = float64(x)
	case int:
		f = float64(x)
	case string:
		p, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: data must be numeric, got %q", ErrShape, x)
		}
		f = p
	case []byte:
		p, err := strconv.ParseFloat(string(x), 64)
		if err != nil {
			return nil, fmt.Errorf("%w: data must be numeric, got %q", ErrShape, x)
		}
		f = p
	default:
		return nil, fmt.Errorf("%w: data must be numeric, got %T", ErrShape, v)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, nil
	}
	return &f, nil
}

func text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.In(pipeline.Taipei).Format(time.RFC3339)
	default:
		return fmt.Sprint(x)
	}
}
