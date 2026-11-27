package cwa

import (
	"context"
	"errors"
	"net/url"

	"dashboard.tw/backend/internal/pipeline"
)

const Datastore = "https://opendata.cwa.gov.tw/api/v1/rest/datastore/"

const FileAPI = "https://opendata.cwa.gov.tw/fileapi/v1/opendataapi/"

func Get(ctx context.Context, env pipeline.Env, endpoint, key string) ([]byte, error) {
	return GetWith(ctx, env, endpoint, key, nil)
}

// GetWith adds extra query parameters such as locationId or timeFrom.
func GetWith(ctx context.Context, env pipeline.Env, endpoint, key string, extra url.Values) ([]byte, error) {
	if key == "" {
		return nil, errors.New("CWA_API_KEY 沒有設定")
	}
	q := url.Values{"format": {"JSON"}}
	for k, v := range extra {
		q[k] = v
	}
	redacted := endpoint + "?" + q.Encode() + "&Authorization=***"
	q.Set("Authorization", key)
	return env.Fetch.Get(ctx, endpoint+"?"+q.Encode(), redacted)
}
