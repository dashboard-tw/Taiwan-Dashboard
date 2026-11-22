package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type healthStatus struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Version  string `json:"version"`
}

func health(db Pinger, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := healthStatus{Status: "ok", Database: "ok", Version: version}
		status := http.StatusOK
		if err := db.Ping(c.Request.Context()); err != nil {
			body.Status = "degraded"
			body.Database = "unavailable"
			status = http.StatusServiceUnavailable
		}
		c.Header("Cache-Control", "no-store")
		writeData(c, status, body, nil)
	}
}
