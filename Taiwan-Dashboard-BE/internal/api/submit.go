package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/workqueue"
)

type submitGate struct {
	limit *limiter
	queue *workqueue.Queue
}

func newSubmitGate() *submitGate {
	q := workqueue.New(2, 50)
	q.Start(context.Background())
	return &submitGate{limit: newLimiter(10, 10), queue: q}
}

func (g *submitGate) run(c *gin.Context, fn func(context.Context) error) (ok bool, err error) {
	key := "ip:" + c.ClientIP()
	if u := currentUser(c); u != nil {
		key = "user:" + strconv.FormatInt(u.ID, 10)
	}
	if allowed, wait := g.limit.allow(key); !allowed {
		c.Header("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		writeError(c, http.StatusTooManyRequests, "rate_limited", "送出太頻繁，請過一分鐘再試")
		return false, nil
	}
	err = g.queue.Do(c.Request.Context(), fn)
	if errors.Is(err, workqueue.ErrBusy) {
		c.Header("Retry-After", "10")
		writeError(c, http.StatusServiceUnavailable, "busy", "目前送出的人很多，請稍後再試")
		return false, nil
	}
	if errors.Is(err, context.Canceled) {
		return false, nil
	}
	return true, err
}
