package hooks

import (
	"context"
	"database/sql"
	"log/slog"
)

const FailStreak = 3

func PipelineFinished(ctx context.Context, db *sql.DB, site, datasetID, name string, runErr error) {
	rows, err := db.QueryContext(ctx, `
		SELECT status FROM app.pipeline_run
		 WHERE dataset_id = $1 AND status <> 'running'
		 ORDER BY started_at DESC, id DESC LIMIT $2`, datasetID, FailStreak+1)
	if err != nil {
		slog.Error("pipeline notify: history", "dataset", datasetID, "error", err)
		return
	}
	var recent []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return
		}
		recent = append(recent, s)
	}
	rows.Close()
	streak := 0
	for _, s := range recent {
		if s != "failed" {
			break
		}
		streak++
	}
	link := site + "/admin/pipeline"
	switch {
	case runErr != nil && streak == FailStreak:
		emitLog(ctx, db, EventPipelineFailed, Message{
			Title: "資料管線連續失敗：" + name, Level: LevelError, URL: link,
			Text:   cut(runErr.Error(), 500),
			Fields: []Field{{"資料集", datasetID}, {"連續失敗", "3 次"}},
		})
	case runErr == nil && len(recent) == FailStreak+1 && allFailed(recent[1:]):
		emitLog(ctx, db, EventPipelineFailed, Message{
			Title: "資料管線恢復正常：" + name, Level: LevelOK, URL: link,
			Fields: []Field{{"資料集", datasetID}},
		})
	}
}

func allFailed(s []string) bool {
	for _, v := range s {
		if v != "failed" {
			return false
		}
	}
	return true
}

func emitLog(ctx context.Context, db *sql.DB, event string, m Message) {
	if err := Emit(ctx, db, event, m); err != nil {
		slog.Error("notify channels", "event", event, "error", err)
	}
}
