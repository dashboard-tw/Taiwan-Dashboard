package alerts

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultClosureURL = "https://www.dgpa.gov.tw/typh/daily/nds.html"

type Closure struct {
	County string    // as written on the page, e.g. 臺北市
	Day    time.Time // the day it applies to, midnight in Taipei
	Status string
	Level  int
}

var (
	reRow      = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	reCell     = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	reTag      = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace    = regexp.MustCompile(`\s+`)
	reUpdated  = regexp.MustCompile(`更新時間[：:]\s*(\d{4}/\d{1,2}/\d{1,2}\s+\d{1,2}:\d{2}:\d{2})`)
	rePageDate = regexp.MustCompile(`(\d{2,3})年\s*(\d{1,2})月\s*(\d{1,2})日\s*天然災害`)
	reBreak    = regexp.MustCompile(`(?i)<br\s*/?>`)
	reNotMet   = regexp.MustCompile(`未達[^。，,]*標準`)
)

func cellText(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	return strings.TrimSpace(reSpace.ReplaceAllString(html.UnescapeString(s), " "))
}

var partial = []string{"部分", "區", "鄉", "鎮", "里", "村", "下午", "晚上", "夜間", "上午", "原住民"}

func ClosureLevel(status string) int {
	status = reNotMet.ReplaceAllString(status, "")
	status = strings.ReplaceAll(status, "停止上班及上課", "停止上班、停止上課")
	if !strings.Contains(status, "停止") {
		return 0
	}
	if strings.Contains(status, "停止上班") && strings.Contains(status, "停止上課") {
		for _, p := range partial {
			if strings.Contains(status, p) {
				return 1
			}
		}
		return 2
	}
	return 1
}

func dayLines(cell string, today time.Time) []Closure {
	var out []Closure
	for _, part := range reBreak.Split(cell, -1) {
		line := cellText(part)
		if line == "" {
			continue
		}
		day := today
		switch {
		case strings.HasPrefix(line, "明天") || strings.HasPrefix(line, "明日"):
			day = today.AddDate(0, 0, 1)
		case strings.HasPrefix(line, "今天") || strings.HasPrefix(line, "今日"):
		case len(out) > 0:
			day = out[len(out)-1].Day
		}
		if n := len(out); n > 0 && out[n-1].Day.Equal(day) {
			out[n-1].Status += " " + line
			continue
		}
		out = append(out, Closure{Day: day, Status: line})
	}
	for i := range out {
		out[i].Level = ClosureLevel(out[i].Status)
	}
	return out
}

func ParseClosures(body []byte, known func(string) bool) (time.Time, []Closure, error) {
	page := string(pipeline.StripBOM(body))
	var updated time.Time
	if m := reUpdated.FindStringSubmatch(page); m != nil {
		t, err := pipeline.ParseTaipeiTime(m[1], "2006/1/2 15:04:05")
		if err == nil {
			updated = t
		}
	}
	today := pipeline.TaipeiDate(updated)
	if m := rePageDate.FindStringSubmatch(page); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		if mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
			today = time.Date(y+1911, time.Month(mo), d, 0, 0, 0, 0, pipeline.Taipei)
		}
	}
	var out []Closure
	for _, row := range reRow.FindAllStringSubmatch(page, -1) {
		cells := reCell.FindAllStringSubmatch(row[1], -1)
		for i := 0; i+1 < len(cells); i++ {
			name := cellText(cells[i][1])
			if !known(name) {
				continue
			}
			for _, c := range dayLines(cells[i+1][1], today) {
				c.County = name
				out = append(out, c)
			}
			break
		}
	}
	if len(out) > 0 && updated.IsZero() {
		return updated, nil, errors.New("找不到頁面的更新時間")
	}
	return updated, out, nil
}

type ClosurePipeline struct {
	URL string
}

func (p *ClosurePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "work_closure",
		Name:         "天然災害停止上班及上課情形",
		SourceAgency: "行政院人事行政總處（資料來源：各縣市政府）",
		SourceURL:    DefaultClosureURL,
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

func (p *ClosurePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	known := func(s string) bool { _, ok := counties.Code(s); return ok && len([]rune(s)) <= 4 }
	updated, list, err := ParseClosures(body, known)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	res.ObserveTime(env.Now())
	if len(list) == 0 {
		return res, nil
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, c := range list {
		code, _ := counties.Code(c.County)
		day := pipeline.DateColumn(c.Day)
		r, err := tx.ExecContext(ctx, `
			UPDATE data.work_closure w SET last_seen_at = now()
			 WHERE w.county_code = $1 AND w.day = $2 AND w.status = $3
			   AND w.announced_at = (SELECT max(announced_at) FROM data.work_closure WHERE county_code = $1 AND day = $2)`,
			code, day, c.Status)
		if err != nil {
			return res, err
		}
		if n, _ := r.RowsAffected(); n > 0 {
			continue
		}
		r, err = tx.ExecContext(ctx, `
			INSERT INTO data.work_closure (county_code, day, announced_at, status, level) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (county_code, day, announced_at) DO UPDATE SET status = EXCLUDED.status, level = EXCLUDED.level,
			    last_seen_at = now()`,
			code, day, updated, c.Status, c.Level)
		if err != nil {
			return res, fmt.Errorf("insert closure %s: %w", c.County, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	return res, tx.Commit()
}
