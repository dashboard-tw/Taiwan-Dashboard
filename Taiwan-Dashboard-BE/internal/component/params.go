package component

import (
	"fmt"
	"strings"
	"time"
)

type Params struct {
	County  *string // nil means national
	From    time.Time
	To      time.Time
	Step    string
	Typhoon *string
	AsOf    time.Time
}

const AsOfSetting = "dashboardtw.as_of"

const AsOfSQL = "coalesce(nullif(current_setting('" + AsOfSetting + "', true), '')::timestamptz, now())"

var clockWords = map[string]string{
	"now":               "(" + AsOfSQL + ")", // only as now()
	"current_timestamp": "(" + AsOfSQL + ")",
	"current_date":      "((" + AsOfSQL + ")::date)",
}

var paramNames = map[string]bool{"county": true, "from": true, "to": true, "step": true, "typhoon": true}

type Compiled struct {
	SQL   string
	names []string // names[i] is bound to $i+1
}

func Compile(query string) (Compiled, error) {
	var b strings.Builder
	index := map[string]int{}
	var names []string

	for i := 0; i < len(query); {
		c := query[i]
		switch {
		case c == '\'' || c == '"':
			end := closing(query, i+1, c)
			b.WriteString(query[i:end])
			i = end
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			end := strings.IndexByte(query[i:], '\n')
			if end < 0 {
				end = len(query) - i
			}
			b.WriteString(query[i : i+end])
			i += end
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			end := strings.Index(query[i+2:], "*/")
			if end < 0 {
				return Compiled{}, fmt.Errorf("unterminated comment")
			}
			b.WriteString(query[i : i+2+end+2])
			i += 2 + end + 2
		case c == '$':
			if i+1 < len(query) && '0' <= query[i+1] && query[i+1] <= '9' {
				return Compiled{}, fmt.Errorf("positional parameters like $1 are not allowed; use :county, :from, :to, :step or :typhoon")
			}
			tag, ok := dollarTag(query, i)
			if !ok {
				b.WriteByte(c)
				i++
				continue
			}
			end := strings.Index(query[i+len(tag):], tag)
			if end < 0 {
				return Compiled{}, fmt.Errorf("unterminated dollar-quoted string")
			}
			stop := i + len(tag) + end + len(tag)
			b.WriteString(query[i:stop])
			i = stop
		case c == ':' && i+1 < len(query) && query[i+1] == ':':
			b.WriteString("::")
			i += 2
		case c == ':' && i+1 < len(query) && isIdentStart(query[i+1]):
			j := i + 1
			for j < len(query) && isIdentPart(query[j]) {
				j++
			}
			name := query[i+1 : j]
			if !paramNames[name] {
				return Compiled{}, fmt.Errorf("unknown parameter :%s (allowed: :county, :from, :to, :step, :typhoon)", name)
			}
			n, seen := index[name]
			if !seen {
				names = append(names, name)
				n = len(names)
				index[name] = n
			}
			fmt.Fprintf(&b, "$%d", n)
			i = j
		case isIdentStart(c) && (i == 0 || !isIdentPart(query[i-1]) && query[i-1] != '.'):
			j := i + 1
			for j < len(query) && isIdentPart(query[j]) {
				j++
			}
			word := strings.ToLower(query[i:j])
			repl, isClock := clockWords[word]
			if isClock && word == "now" {
				end, ok := emptyParens(query, j)
				if !ok {
					isClock = false
				}
				j = end
			}
			if isClock {
				b.WriteString(repl)
			} else {
				b.WriteString(query[i:j])
			}
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return Compiled{SQL: b.String(), names: names}, nil
}

func (c Compiled) Uses(name string) bool {
	for _, n := range c.names {
		if n == name {
			return true
		}
	}
	return false
}

func (c Compiled) Args(p Params) []any {
	args := make([]any, len(c.names))
	for i, n := range c.names {
		switch n {
		case "county":
			if p.County != nil {
				args[i] = *p.County
			}
		case "from":
			args[i] = p.From
		case "to":
			args[i] = p.To
		case "step":
			args[i] = p.Step
		case "typhoon":
			if p.Typhoon != nil {
				args[i] = *p.Typhoon
			}
		}
	}
	return args
}

func closing(s string, start int, quote byte) int {
	for i := start; i < len(s); i++ {
		if s[i] == quote {
			if i+1 < len(s) && s[i+1] == quote {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(s)
}

func emptyParens(s string, i int) (int, bool) {
	j := skipSpace(s, i)
	if j >= len(s) || s[j] != '(' {
		return i, false
	}
	j = skipSpace(s, j+1)
	if j >= len(s) || s[j] != ')' {
		return i, false
	}
	return j + 1, true
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	return i
}

func dollarTag(s string, i int) (string, bool) {
	j := i + 1
	for j < len(s) && isIdentPart(s[j]) {
		j++
	}
	if j < len(s) && s[j] == '$' && (j == i+1 || isIdentStart(s[i+1])) {
		return s[i : j+1], true
	}
	return "", false
}

func isIdentStart(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || ('0' <= c && c <= '9')
}

var Ranges = map[string]time.Duration{
	"current":  24 * time.Hour,
	"day":      24 * time.Hour,
	"week":     7 * 24 * time.Hour,
	"month":    30 * 24 * time.Hour,
	"quarter":  91 * 24 * time.Hour,
	"halfyear": 182 * 24 * time.Hour,
	"year":     365 * 24 * time.Hour,
	"twoyear":  730 * 24 * time.Hour,
	"fiveyear": 1826 * 24 * time.Hour,
}

const MaxRange = 1827 * 24 * time.Hour

func StepFor(from, to time.Time) string {
	d := to.Sub(from)
	switch {
	case d <= 24*time.Hour:
		return "hour"
	case d <= 31*24*time.Hour:
		return "day"
	case d <= 92*24*time.Hour:
		return "week"
	case d <= 731*24*time.Hour:
		return "month"
	default:
		return "year"
	}
}
