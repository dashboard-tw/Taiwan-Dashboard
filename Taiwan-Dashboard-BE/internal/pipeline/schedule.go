package pipeline

import "time"

type Schedule struct {
	Every  time.Duration
	Offset time.Duration
}

func (s Schedule) Next(now time.Time) time.Time {
	local := now.In(Taipei)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, Taipei)
	elapsed := local.Sub(midnight) - s.Offset
	slots := elapsed / s.Every
	if elapsed < 0 {
		slots = -1
	}
	next := midnight.Add(s.Offset + (slots+1)*s.Every)
	for !next.After(now) {
		next = next.Add(s.Every)
	}
	return next
}
