package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"time"
)

type Fetcher struct {
	Client    *http.Client
	UserAgent string
	MaxBytes  int64
	Retries   int
	Backoff   time.Duration
}

func NewFetcher(version string) *Fetcher {
	return &Fetcher{
		Client:    &http.Client{Timeout: 60 * time.Second, Transport: newTransport()},
		UserAgent: "dashboard.tw-worker/" + version + " (+https://dashboard.tw)",
		MaxBytes:  50 << 20,
		Retries:   2,
		Backoff:   5 * time.Second,
	}
}

type HTTPError struct {
	URL    string
	Status int
}

func (e *HTTPError) Error() string { return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.Status) }

func (f *Fetcher) Get(ctx context.Context, url, redactedURL string) ([]byte, error) {
	return f.retry(ctx, func() ([]byte, error) { return f.get(ctx, url, redactedURL, "") })
}

func (f *Fetcher) GetRange(ctx context.Context, url string, offset, length int64) ([]byte, error) {
	if offset < 0 || length <= 0 || length > f.MaxBytes {
		return nil, fmt.Errorf("GET %s: invalid range %d+%d", url, offset, length)
	}
	byteRange := fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	return f.retry(ctx, func() ([]byte, error) { return f.get(ctx, url, url, byteRange) })
}

func (f *Fetcher) retry(ctx context.Context, get func() ([]byte, error)) ([]byte, error) {
	var lastErr error
	wait := f.Backoff
	for attempt := 0; attempt <= f.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			wait *= 2
		}
		body, err := get()
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable(err) {
			break
		}
	}
	return nil, lastErr
}

func (f *Fetcher) get(ctx context.Context, url, redactedURL, byteRange string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", redactedURL, err)
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "application/json, text/csv, */*")
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", redactedURL, unwrapURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, &HTTPError{URL: redactedURL, Status: resp.StatusCode}
	}
	if byteRange != "" && resp.StatusCode != http.StatusPartialContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("GET %s: server ignored the byte range (HTTP %d)", redactedURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", redactedURL, err)
	}
	if int64(len(body)) > f.MaxBytes {
		return nil, fmt.Errorf("GET %s: response larger than %d bytes", redactedURL, f.MaxBytes)
	}
	if byteRange != "" {
		return body, nil // binary
	}
	return StripBOM(body), nil
}

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Status >= 500 || httpErr.Status == http.StatusTooManyRequests
	}
	return true // network errors and timeouts
}

func unwrapURLError(err error) error {
	var ue *neturl.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err
	}
	return err
}
