package pipelinetest

import (
	"bytes"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

type Server struct {
	*httptest.Server
	mu     sync.Mutex
	routes map[string]string
	status map[string]int
}

func NewServer(t *testing.T, routes map[string]string) *Server {
	t.Helper()
	s := &Server{routes: map[string]string{}, status: map[string]int{}}
	for k, v := range routes {
		s.routes[k] = v
	}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		file, ok := s.routes[r.URL.Path]
		code := s.status[r.URL.Path]
		s.mu.Unlock()
		if code != 0 {
			http.Error(w, "fixture error", code)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, file, time.Time{}, bytes.NewReader(b))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *Server) Set(path, file string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[path] = file
	delete(s.status, path)
}

func (s *Server) Fail(path string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[path] = status
}

func (s *Server) Env(db *sql.DB, now time.Time) pipeline.Env {
	return pipeline.Env{
		DB:     db,
		Fetch:  &pipeline.Fetcher{Client: s.Client(), UserAgent: "test", MaxBytes: 10 << 20},
		Now:    func() time.Time { return now },
		Logger: slog.New(slog.DiscardHandler),
	}
}

func Count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}
