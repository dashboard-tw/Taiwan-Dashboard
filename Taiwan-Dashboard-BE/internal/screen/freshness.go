package screen

import (
	"time"

	"dashboard.tw/backend/internal/component"
)

const PauseAfterMin = 6 * time.Hour

func sourcePaused(s component.Source, now time.Time) (time.Time, bool) {
	if s.Status == "pending" || s.LastIngestedAt == nil || s.StaleAfterSeconds <= 0 {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, *s.LastIngestedAt)
	if err != nil {
		return time.Time{}, false
	}
	limit := max(time.Duration(s.StaleAfterSeconds)*time.Second, PauseAfterMin)
	return at, now.Sub(at) > limit
}

func pausedAsOf(sources []component.Source, now time.Time) (time.Time, bool) {
	var fastest int64
	for _, s := range sources {
		if s.UpdateFreqSeconds > 0 && (fastest == 0 || s.UpdateFreqSeconds < fastest) {
			fastest = s.UpdateFreqSeconds
		}
	}
	var asOf time.Time
	found := false
	for _, s := range sources {
		if fastest > 0 && s.UpdateFreqSeconds != fastest {
			continue
		}
		at, paused := sourcePaused(s, now)
		if !paused {
			return time.Time{}, false
		}
		if !found || at.Before(asOf) {
			asOf = at
		}
		found = true
	}
	return asOf, found
}
