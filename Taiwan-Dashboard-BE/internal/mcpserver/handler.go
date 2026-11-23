package mcpserver

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

const (
	PerMinute     = 120
	Burst         = 60
	MaxConcurrent = 64
)

type HTTPOptions struct {
	Verify              auth.TokenVerifier
	ResourceMetadataURL string
	OnToolCall          func(ctx context.Context, userID, tool string)
}

func Handler(srv *mcp.Server, o HTTPOptions) http.Handler {
	if o.OnToolCall != nil {
		srv.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				if method == "tools/call" {
					if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
						user := ""
						if ex := req.GetExtra(); ex != nil && ex.TokenInfo != nil {
							user = ex.TokenInfo.UserID
						}
						o.OnToolCall(ctx, user, p.Name)
					}
				}
				return next(ctx, method, req)
			}
		})
	}
	get := func(*http.Request) *mcp.Server { return srv }
	streamable := mcp.NewStreamableHTTPHandler(get, &mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true})
	sse := mcp.NewSSEHandler(get, &mcp.SSEOptions{DisableLocalhostProtection: true, MaxRequestBodyBytes: 1 << 20})

	mux := http.NewServeMux()
	mux.Handle("/mcp", streamable)
	mux.Handle("/mcp/sse", sse)

	protected := auth.RequireBearerToken(o.Verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL:    o.ResourceMetadataURL,
		AllowMissingExpiration: true,
	})(limit(mux))
	return cors(concurrency(challenge(protected)))
}

type challengeWriter struct{ http.ResponseWriter }

func (w challengeWriter) WriteHeader(code int) {
	if code == http.StatusUnauthorized && w.Header().Get("WWW-Authenticate") == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="dashboard.tw"`)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w challengeWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w challengeWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func challenge(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(challengeWriter{w}, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Mcp-Session-Id, MCP-Protocol-Version, Mcp-Method, Mcp-Name, Last-Event-ID")
		h.Set("Access-Control-Expose-Headers", "Mcp-Session-Id, WWW-Authenticate")
		h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			h.Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.Contains(r.URL.RawQuery, "access_token=") {
			http.Error(w, "access token must be sent in the Authorization header", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func concurrency(next http.Handler) http.Handler {
	slots := make(chan struct{}, MaxConcurrent)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", "5")
			http.Error(w, "server busy, retry shortly", http.StatusServiceUnavailable)
		}
	})
}

type userLimiter struct {
	mu    sync.Mutex
	users map[string]*entry
	swept time.Time
}

type entry struct {
	l    *rate.Limiter
	seen time.Time
}

func (u *userLimiter) allow(user string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := time.Now()
	if now.Sub(u.swept) > 10*time.Minute {
		for k, e := range u.users {
			if now.Sub(e.seen) > 10*time.Minute {
				delete(u.users, k)
			}
		}
		u.swept = now
	}
	e := u.users[user]
	if e == nil {
		e = &entry{l: rate.NewLimiter(rate.Limit(float64(PerMinute)/60), Burst)}
		u.users[user] = e
	}
	e.seen = now
	return e.l.Allow()
}

func limit(next http.Handler) http.Handler {
	u := &userLimiter{users: map[string]*entry{}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := ""
		if ti := auth.TokenInfoFromContext(r.Context()); ti != nil {
			user = ti.UserID
		}
		if !u.allow(user) {
			w.Header().Set("Retry-After", "30")
			http.Error(w, "too many requests, slow down", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
