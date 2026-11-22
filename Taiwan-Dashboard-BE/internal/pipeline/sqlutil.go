package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func Placeholders(n int) string {
	p := make([]string, n)
	for i := range p {
		p[i] = fmt.Sprintf("$%d", i+1)
	}
	return strings.Join(p, ", ")
}

func SetExcluded(cols []string) string {
	s := make([]string, len(cols))
	for i, c := range cols {
		s[i] = c + " = EXCLUDED." + c
	}
	return strings.Join(s, ", ")
}

func NullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

const BatchRows = 500

func InsertRows(ctx context.Context, tx *sql.Tx, head string, rows [][]any, tail string) (int, error) {
	n := 0
	for start := 0; start < len(rows); start += BatchRows {
		batch := rows[start:min(start+BatchRows, len(rows))]
		var b strings.Builder
		b.WriteString(head)
		var args []any
		for i, r := range batch {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(" (")
			for j, v := range r {
				if j > 0 {
					b.WriteString(", ")
				}
				args = append(args, v)
				fmt.Fprintf(&b, "$%d", len(args))
			}
			b.WriteString(")")
		}
		b.WriteString(" ")
		b.WriteString(tail)
		res, err := tx.ExecContext(ctx, b.String(), args...)
		if err != nil {
			return n, err
		}
		affected, _ := res.RowsAffected()
		n += int(affected)
	}
	return n, nil
}
