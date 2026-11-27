package api

import (
	"database/sql"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 頁尾的瀏覽人次與在線人數。
// 瀏覽人次：瀏覽器每次開啟網站送一次 new=true，存在資料庫，永久累計。
// 在線人數：開著的網頁每 2 分鐘回報一次，5 分鐘內有回報的算在線，只放在記憶體。
// 訪客代碼是瀏覽器產生的隨機字串，不存 IP，也不記錄看了哪些頁面。

const onlineWindow = 5 * time.Minute

var visitorIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,40}$`)

type presence struct {
	db   *sql.DB
	now  func() time.Time
	mu   sync.Mutex
	seen map[string]time.Time
}

func newPresence(db *sql.DB) *presence {
	return &presence{db: db, now: time.Now, seen: map[string]time.Time{}}
}

func (p *presence) touch(id string, isNew bool) (online int, count bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	for k, t := range p.seen {
		if now.Sub(t) > onlineWindow {
			delete(p.seen, k)
		}
	}
	_, known := p.seen[id]
	p.seen[id] = now
	// 同一個代碼在線時重送 new 不重複計算
	return len(p.seen), isNew && !known
}

func (p *presence) handle(c *gin.Context) {
	var in struct {
		ID  string `json:"id"`
		New bool   `json:"new"`
	}
	if !bindJSON(c, &in) {
		return
	}
	if !visitorIDRe.MatchString(in.ID) {
		writeError(c, http.StatusBadRequest, "invalid_id", "訪客代碼格式不正確")
		return
	}
	online, count := p.touch(in.ID, in.New)
	ctx := c.Request.Context()
	if count {
		if _, err := p.db.ExecContext(ctx, `
			INSERT INTO app.site_visit_daily AS v (day, visits) VALUES ((now() AT TIME ZONE 'Asia/Taipei')::date, 1)
			ON CONFLICT (day) DO UPDATE SET visits = v.visits + 1`); err != nil {
			slog.Error("count site visit", "error", err)
		}
	}
	var visits int64
	if err := p.db.QueryRowContext(ctx, `SELECT coalesce(sum(visits), 0) FROM app.site_visit_daily`).Scan(&visits); err != nil {
		slog.Error("read site visits", "error", err)
	}
	noStore(c)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"visits": visits, "online": online}})
}
