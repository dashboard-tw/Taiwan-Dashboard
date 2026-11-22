package pipeline

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

type Counties struct {
	exact    map[string]string
	prefixes []countyName
}

type countyName struct{ name, code string }

func NormalizeCountyText(s string) string {
	s = strings.ReplaceAll(s, "台", "臺")
	s = strings.ReplaceAll(s, "巿", "市")
	return strings.Join(strings.Fields(s), "")
}

func NewCounties(names map[string][]string) *Counties {
	c := &Counties{exact: map[string]string{}}
	for code, list := range names {
		for i, n := range list {
			n = NormalizeCountyText(n)
			c.exact[n] = code
			if i == 0 {
				c.prefixes = append(c.prefixes, countyName{n, code})
			}
		}
	}
	sort.Slice(c.prefixes, func(i, j int) bool {
		if len(c.prefixes[i].name) != len(c.prefixes[j].name) {
			return len(c.prefixes[i].name) > len(c.prefixes[j].name)
		}
		return c.prefixes[i].name < c.prefixes[j].name
	})
	return c
}

func LoadCounties(ctx context.Context, db *sql.DB) (*Counties, error) {
	rows, err := db.QueryContext(ctx, `SELECT code, name, aliases FROM ref.county`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := map[string][]string{}
	for rows.Next() {
		var code, name string
		var aliases []string
		if err := rows.Scan(&code, &name, &aliases); err != nil {
			return nil, err
		}
		names[code] = append([]string{name}, aliases...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewCounties(names), nil
}

func (c *Counties) Code(name string) (string, bool) {
	code, ok := c.exact[NormalizeCountyText(name)]
	return code, ok
}

func (c *Counties) CodeFromAddress(addr string) (string, bool) {
	addr = NormalizeCountyText(addr)
	for _, p := range c.prefixes {
		if strings.HasPrefix(addr, p.name) {
			return p.code, true
		}
	}
	return "", false
}
