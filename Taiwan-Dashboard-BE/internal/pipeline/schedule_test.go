package pipeline

import (
	"testing"
	"time"
)

func at(h, m int) time.Time { return time.Date(2026, 10, 20, h, m, 0, 0, Taipei) }

func TestScheduleNext(t *testing.T) {
	cases := []struct {
		name string
		s    Schedule
		now  time.Time
		want time.Time
	}{
		{"hourly before offset", Schedule{time.Hour, 25 * time.Minute}, at(15, 10), at(15, 25)},
		{"hourly after offset", Schedule{time.Hour, 25 * time.Minute}, at(15, 30), at(16, 25)},
		{"hourly exactly on slot", Schedule{time.Hour, 25 * time.Minute}, at(15, 25), at(16, 25)},
		{"ten minutes", Schedule{10 * time.Minute, 3 * time.Minute}, at(15, 4), at(15, 13)},
		{"daily later today", Schedule{24 * time.Hour, 10 * time.Hour}, at(9, 0), at(10, 0)},
		{"daily tomorrow", Schedule{24 * time.Hour, 10 * time.Hour}, at(11, 0), at(10, 0).AddDate(0, 0, 1)},
		{"just after midnight", Schedule{time.Hour, 25 * time.Minute}, at(0, 5), at(0, 25)},
	}
	for _, c := range cases {
		if got := c.s.Next(c.now); !got.Equal(c.want) {
			t.Errorf("%s: Next(%s) = %s, want %s", c.name, c.now.Format(time.DateTime), got.In(Taipei).Format(time.DateTime), c.want.Format(time.DateTime))
		}
	}
}

func TestScheduleNextUsesTaipeiTime(t *testing.T) {
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	got := Schedule{24 * time.Hour, 10 * time.Hour}.Next(now)
	if want := at(10, 0); !got.Equal(want) {
		t.Errorf("got %s, want %s", got.In(Taipei), want)
	}
}
