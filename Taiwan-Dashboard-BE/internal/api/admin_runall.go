package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/pipeline"
)

type runAllStep struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Status   string   `json:"status"` // pending, running, ok, warning, failed, skipped
	Fetched  int      `json:"rows_fetched"`
	Inserted int      `json:"rows_inserted"`
	Message  string   `json:"message,omitempty"`
	Seconds  *float64 `json:"seconds,omitempty"`
}

type runAllState struct {
	Running    bool         `json:"running"`
	StartedAt  *time.Time   `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at"`
	Done       int          `json:"done"`
	Total      int          `json:"total"`
	Current    string       `json:"current,omitempty"` // dataset id being run
	Steps      []runAllStep `json:"steps"`
}

type runAll struct {
	mu    sync.Mutex
	state runAllState
}

func (j *runAll) snapshot() runAllState {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := j.state
	s.Steps = append([]runAllStep{}, j.state.Steps...)
	if s.Steps == nil {
		s.Steps = []runAllStep{}
	}
	return s
}

func (j *runAll) start(ids []string, names map[string]string, now time.Time) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state.Running {
		return false
	}
	steps := make([]runAllStep, len(ids))
	for i, id := range ids {
		name := names[id]
		if name == "" {
			name = id
		}
		steps[i] = runAllStep{ID: id, Name: name, Status: "pending"}
	}
	j.state = runAllState{Running: true, StartedAt: &now, Total: len(ids), Steps: steps}
	return true
}

func (j *runAll) update(i int, f func(s *runAllStep)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	f(&j.state.Steps[i])
	if j.state.Steps[i].Status == "running" {
		j.state.Current = j.state.Steps[i].ID
	} else {
		j.state.Done++
		j.state.Current = ""
	}
}

func (j *runAll) finish(now time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.state.Running = false
	j.state.Current = ""
	j.state.FinishedAt = &now
}

func (j *runAll) run(ctx context.Context, run func(ctx context.Context, id string) (pipeline.Result, error)) {
	defer func() { j.finish(time.Now()) }()
	steps := j.snapshot().Steps
	for i, s := range steps {
		j.update(i, func(st *runAllStep) { st.Status = "running" })
		began := time.Now()
		res, err := run(ctx, s.ID)
		secs := time.Since(began).Seconds()
		j.update(i, func(st *runAllStep) {
			st.Seconds = &secs
			st.Fetched, st.Inserted = res.RowsFetched, res.RowsInserted
			switch {
			case errors.Is(err, pipeline.ErrAlreadyRunning):
				st.Status, st.Message = "skipped", "worker 正在抓這個資料集，略過"
			case errors.Is(err, pipeline.ErrNotConfigured):
				st.Status, st.Message = "skipped", pipeline.Truncate(err.Error(), 200)
			case err != nil:
				st.Status, st.Message = "failed", pipeline.Truncate(err.Error(), 300)
			case len(res.Warnings) > 0:
				st.Status, st.Message = "warning", pipeline.Truncate(res.Warnings[0], 200)
			default:
				st.Status = "ok"
			}
		})
	}
}

func (h adminHandlers) runAllStatus(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	writeData(c, http.StatusOK, h.all.snapshot(), nil)
}

func (h adminHandlers) startRunAll(c *gin.Context) {
	u, ok := mustAdmin(c)
	if !ok {
		return
	}
	if h.run == nil || len(h.order) == 0 {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "這台伺服器連到其他主機的資料庫，不負責抓資料，請在資料庫所在的主機上執行")
		return
	}
	names := map[string]string{}
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT id, name FROM app.dataset`)
	if err == nil {
		for rows.Next() {
			var id, name string
			if rows.Scan(&id, &name) == nil {
				names[id] = name
			}
		}
		rows.Close()
	}
	if !h.all.start(h.order, names, time.Now()) {
		writeError(c, http.StatusConflict, "already_running", "已經在全部更新中，請等這一次跑完")
		return
	}
	if err := h.store.Audit(context.WithoutCancel(c.Request.Context()), &u.ID, "dataset.run_all", "dataset:*",
		map[string]any{"datasets": len(h.order)}, c.ClientIP()); err != nil {
		slog.Error("audit run all", "error", err)
	}
	go h.all.run(context.Background(), h.run)
	writeData(c, http.StatusAccepted, h.all.snapshot(), nil)
}
