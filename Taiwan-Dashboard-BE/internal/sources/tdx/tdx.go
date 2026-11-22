package tdx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultTokenURL = "https://tdx.transportdata.tw/auth/realms/TDXConnect/protocol/openid-connect/token"
	DefaultAPI      = "https://tdx.transportdata.tw/api/basic/"
	PerMinute       = 5
)

type Client struct {
	ClientID, ClientSecret string
	TokenURL               string // empty uses DefaultTokenURL
	API                    string // empty uses DefaultAPI
	Limiter                *Limiter

	mu        sync.Mutex
	token     string
	expires   time.Time
	failUntil time.Time
	failDelay time.Duration
	failErr   error
}

const (
	tokenRetryMin = 30 * time.Minute
	tokenRetryMax = 6 * time.Hour
)

var ErrNoKey = fmt.Errorf("%w：未設定 TDX_CLIENT_ID 與 TDX_CLIENT_SECRET", pipeline.ErrNotConfigured)

func NewClient(id, secret string) *Client {
	return &Client{ClientID: id, ClientSecret: secret, Limiter: NewLimiter(PerMinute, time.Minute)}
}

func (c *Client) api() string {
	if c.API != "" {
		return c.API
	}
	return DefaultAPI
}

func (c *Client) accessToken(ctx context.Context, env pipeline.Env, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.token != "" && env.Now().Before(c.expires.Add(-time.Hour)) {
		return c.token, nil
	}
	if env.Now().Before(c.failUntil) {
		return "", fmt.Errorf("%w，%s 後再試", c.failErr, c.failUntil.In(pipeline.Taipei).Format("15:04"))
	}
	tok, err := c.requestToken(ctx, env)
	if err != nil {
		c.failDelay = min(max(tokenRetryMin, c.failDelay*2), tokenRetryMax)
		c.failUntil = env.Now().Add(c.failDelay)
		c.failErr = err
		return "", err
	}
	c.failDelay, c.failUntil, c.failErr = 0, time.Time{}, nil
	return tok, nil
}

func (c *Client) requestToken(ctx context.Context, env pipeline.Env) (string, error) {
	tokenURL := c.TokenURL
	if tokenURL == "" {
		tokenURL = DefaultTokenURL
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", env.Fetch.UserAgent)
	resp, err := env.Fetch.Client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return "", fmt.Errorf("TDX token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("TDX token: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			return "", errors.New("TDX token: HTTP 429（取得 token 太頻繁）")
		case http.StatusBadRequest, http.StatusUnauthorized:
			return "", fmt.Errorf("TDX token: HTTP %d（可能是本月額度用完，或 Client Id、Client Secret 有誤）", resp.StatusCode)
		}
		return "", fmt.Errorf("TDX token: HTTP %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("TDX token: 回應沒有 access_token")
	}
	if tok.ExpiresIn <= 0 {
		tok.ExpiresIn = 3600
	}
	c.token = tok.AccessToken
	c.expires = env.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return c.token, nil
}

func (c *Client) Get(ctx context.Context, env pipeline.Env, path string, query url.Values) ([]byte, error) {
	if c.ClientID == "" || c.ClientSecret == "" {
		return nil, ErrNoKey
	}
	if query == nil {
		query = url.Values{}
	}
	query.Set("$format", "JSON")
	u := c.api() + path + "?" + query.Encode()
	renewed, waited := false, false
	for {
		token, err := c.accessToken(ctx, env, renewed)
		if err != nil {
			return nil, err
		}
		if err := c.Limiter.Wait(ctx); err != nil {
			return nil, err
		}
		body, status, reset, err := c.get(ctx, env, u, token)
		switch {
		case err != nil:
			return nil, err
		case status == http.StatusUnauthorized && !renewed:
			renewed = true
			continue
		case status == http.StatusTooManyRequests && !waited:
			waited = true
			if reset == 0 {
				reset = c.Limiter.Per
			}
			c.Limiter.Block(reset)
			continue
		case status < 200 || status > 299:
			return nil, &pipeline.HTTPError{URL: u, Status: status}
		}
		return body, nil
	}
}

func (c *Client) get(ctx context.Context, env pipeline.Env, u, token string) ([]byte, int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", env.Fetch.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := env.Fetch.Client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, 0, 0, fmt.Errorf("GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	var reset time.Duration
	if s, err := strconv.Atoi(resp.Header.Get("RateLimit-Reset")); err == nil && s >= 0 && s <= 120 {
		reset = time.Duration(s+1) * time.Second
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, resp.StatusCode, reset, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, env.Fetch.MaxBytes+1))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read %s: %w", u, err)
	}
	if int64(len(body)) > env.Fetch.MaxBytes {
		return nil, 0, 0, fmt.Errorf("GET %s: response larger than %d bytes", u, env.Fetch.MaxBytes)
	}
	return pipeline.StripBOM(body), resp.StatusCode, reset, nil
}

type Limiter struct {
	N   int
	Per time.Duration

	mu    sync.Mutex
	calls []time.Time
	until time.Time // blocked after a 429
	now   func() time.Time
}

func NewLimiter(n int, per time.Duration) *Limiter {
	return &Limiter{N: n, Per: per, now: time.Now}
}

func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := l.now()
		kept := l.calls[:0]
		for _, t := range l.calls {
			if now.Sub(t) < l.Per {
				kept = append(kept, t)
			}
		}
		l.calls = kept
		var wait time.Duration
		switch {
		case now.Before(l.until):
			wait = l.until.Sub(now)
		case len(l.calls) >= l.N:
			wait = l.calls[0].Add(l.Per + time.Second).Sub(now)
		default:
			l.calls = append(l.calls, now)
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *Limiter) Block(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until := l.now().Add(d); until.After(l.until) {
		l.until = until
	}
}
