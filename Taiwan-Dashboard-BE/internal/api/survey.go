package api

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/db"
)

const APITermsVersion = "2026-11-22"

var (
	surveyIdentities = []string{"individual", "student", "media", "government", "nonprofit", "company", "other"}
	surveyPurposes   = []string{"research", "journalism", "app", "analysis", "teaching", "personal", "other"}
	surveyTopics     = []string{"aqi", "reservoir", "power", "outage", "earthquake", "weather", "warning", "traffic", "resources", "population", "tourism", "rail", "bike", "safety", "family", "sustain", "agri", "food", "fire", "housing", "disease", "aviation", "economy", "animal",
		"forecast", "health", "ocean", "flood", "fuel", "power-use", "education"}
)

type survey struct {
	Identity     string    `json:"identity"`
	Purpose      string    `json:"purpose"`
	AppName      string    `json:"app_name"`
	AppURL       string    `json:"app_url"`
	Topics       []string  `json:"topics"`
	Suggestion   string    `json:"suggestion"`
	WantsPush    *string   `json:"wants_push"`
	TermsVersion string    `json:"terms_version"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func loadSurvey(tx *sql.Tx, c *gin.Context) (*survey, error) {
	var s survey
	m := pgtype.NewMap()
	err := tx.QueryRowContext(c.Request.Context(), `
		SELECT identity, purpose, app_name, app_url, topics, suggestion, wants_push, terms_version, updated_at
		  FROM app.api_survey`).Scan(&s.Identity, &s.Purpose, &s.AppName, &s.AppURL, m.SQLScanner(&s.Topics),
		&s.Suggestion, &s.WantsPush, &s.TermsVersion, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

type surveyInput struct {
	Identity   string   `json:"identity"`
	Purpose    string   `json:"purpose"`
	AppName    string   `json:"app_name"`
	AppURL     string   `json:"app_url"`
	Topics     []string `json:"topics"`
	Suggestion string   `json:"suggestion"`
	WantsPush  string   `json:"wants_push"`
	AgreeTerms bool     `json:"agree_terms"`
}

var surveyPush = []string{"yes", "maybe", "no"}

func (in *surveyInput) validate(c *gin.Context) bool {
	in.AppName = strings.TrimSpace(in.AppName)
	in.AppURL = strings.TrimSpace(in.AppURL)
	in.Suggestion = strings.TrimSpace(in.Suggestion)
	fail := func(code, msg string) bool {
		writeError(c, http.StatusBadRequest, code, msg)
		return false
	}
	switch {
	case !in.AgreeTerms:
		return fail("terms_required", "請先閱讀並同意 API 使用規範")
	case !slices.Contains(surveyIdentities, in.Identity):
		return fail("invalid_identity", "請選擇身分類別")
	case !slices.Contains(surveyPurposes, in.Purpose):
		return fail("invalid_purpose", "請選擇資料用途")
	case len(in.Topics) == 0:
		return fail("invalid_topics", "請至少選擇一個關注的資料")
	case !slices.Contains(surveyPush, in.WantsPush):
		return fail("invalid_wants_push", "請選擇是否希望新增 MQTT 即時推送")
	case len([]rune(in.AppName)) > 50:
		return fail("invalid_app_name", "應用名稱最多 50 個字")
	case len([]rune(in.Suggestion)) > 500:
		return fail("invalid_suggestion", "建議最多 500 個字")
	case len(in.AppURL) > 200:
		return fail("invalid_app_url", "網址太長")
	}
	seen := map[string]bool{}
	for _, t := range in.Topics {
		if !slices.Contains(surveyTopics, t) || seen[t] {
			return fail("invalid_topics", "關注的資料有無法辨識的項目，請重新選擇")
		}
		seen[t] = true
	}
	if in.AppURL != "" {
		u, err := url.Parse(in.AppURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fail("invalid_app_url", "網址格式錯誤，請以 https:// 開頭")
		}
	}
	return true
}

func (h meHandlers) saveSurvey(c *gin.Context) {
	var in surveyInput
	if !bindJSON(c, &in) || !in.validate(c) {
		return
	}
	err := h.asUser(c, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(c.Request.Context(), `
			INSERT INTO app.api_survey (user_id, identity, purpose, app_name, app_url, topics, suggestion, wants_push, terms_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (user_id) DO UPDATE SET
			    identity = EXCLUDED.identity, purpose = EXCLUDED.purpose, app_name = EXCLUDED.app_name,
			    app_url = EXCLUDED.app_url, topics = EXCLUDED.topics, suggestion = EXCLUDED.suggestion,
			    wants_push = EXCLUDED.wants_push, terms_version = EXCLUDED.terms_version,
			    agreed_at = CASE WHEN app.api_survey.terms_version = EXCLUDED.terms_version
			                     THEN app.api_survey.agreed_at ELSE now() END,
			    updated_at = now()`,
			h.user(c).ID, in.Identity, in.Purpose, in.AppName, in.AppURL, in.Topics, in.Suggestion, in.WantsPush, APITermsVersion)
		return err
	})
	if err != nil {
		meFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h meHandlers) surveyDone(c *gin.Context) (bool, error) {
	var done bool
	err := h.asUser(c, func(tx *sql.Tx) error {
		// 開發帳號不用填使用調查
		return tx.QueryRowContext(c.Request.Context(),
			`SELECT EXISTS (SELECT 1 FROM app.api_survey WHERE terms_version = $1)
			     OR EXISTS (SELECT 1 FROM app.users WHERE id = $2 AND is_dev)`, APITermsVersion, h.user(c).ID).Scan(&done)
	})
	return done, err
}

type countRow struct {
	Value  string `json:"value"`
	Count  int    `json:"count"`
	Active int    `json:"active"`
}

type surveyNote struct {
	UpdatedAt  time.Time `json:"updated_at"`
	Identity   string    `json:"identity"`
	Purpose    string    `json:"purpose"`
	Topics     []string  `json:"topics"`
	AppName    string    `json:"app_name"`
	AppURL     string    `json:"app_url"`
	Suggestion string    `json:"suggestion"`
	Active     bool      `json:"active"`
}

func (h adminHandlers) apiSurvey(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	ctx := c.Request.Context()
	out := gin.H{}
	err := db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		var total, active, keys int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*), count(*) FILTER (WHERE EXISTS (SELECT 1 FROM app.api_key k WHERE k.user_id = s.user_id AND k.revoked_at IS NULL AND k.kind = 'client')),
			       (SELECT count(*) FROM app.api_key WHERE revoked_at IS NULL AND kind = 'client' AND id NOT IN `+auth.DevKeysSQL+`)
			  FROM app.api_survey s`).Scan(&total, &active, &keys); err != nil {
			return err
		}
		out["respondents"], out["active_respondents"], out["active_keys"] = total, active, keys
		counts := func(query string) ([]countRow, error) {
			rows, err := tx.QueryContext(ctx, query)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			list := []countRow{}
			for rows.Next() {
				var r countRow
				if err := rows.Scan(&r.Value, &r.Count, &r.Active); err != nil {
					return nil, err
				}
				list = append(list, r)
			}
			return list, rows.Err()
		}
		for key, query := range map[string]string{
			"identity":   surveyCounts(`s.identity`),
			"purpose":    surveyCounts(`s.purpose`),
			"topics":     surveyCounts(`t`),
			"wants_push": surveyCounts(`coalesce(s.wants_push, 'unanswered')`),
		} {
			list, err := counts(query)
			if err != nil {
				return err
			}
			out[key] = list
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT s.updated_at, s.identity, s.purpose, s.topics, s.app_name, s.app_url, s.suggestion,
			       EXISTS (SELECT 1 FROM app.api_key k WHERE k.user_id = s.user_id AND k.revoked_at IS NULL AND k.kind = 'client')
			  FROM app.api_survey s
			 WHERE s.app_name <> '' OR s.app_url <> '' OR s.suggestion <> ''
			 ORDER BY s.updated_at DESC LIMIT 100`)
		if err != nil {
			return err
		}
		defer rows.Close()
		m := pgtype.NewMap()
		notes := []surveyNote{}
		for rows.Next() {
			var n surveyNote
			if err := rows.Scan(&n.UpdatedAt, &n.Identity, &n.Purpose, m.SQLScanner(&n.Topics),
				&n.AppName, &n.AppURL, &n.Suggestion, &n.Active); err != nil {
				return err
			}
			notes = append(notes, n)
		}
		out["notes"] = notes
		return rows.Err()
	})
	if err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

func surveyCounts(column string) string {
	topics := ""
	if column == "t" {
		topics = "CROSS JOIN LATERAL unnest(s.topics) t"
	}
	return `SELECT ` + column + `, count(*),
	               count(*) FILTER (WHERE EXISTS (SELECT 1 FROM app.api_key k WHERE k.user_id = s.user_id AND k.revoked_at IS NULL AND k.kind = 'client'))
	          FROM app.api_survey s
	          ` + topics + `
	         GROUP BY 1 ORDER BY 2 DESC, 1`
}
