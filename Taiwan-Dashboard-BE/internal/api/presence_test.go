package api

import (
	"testing"
	"time"
)

func TestPresenceCountsOncePerVisit(t *testing.T) {
	now := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	p := &presence{now: func() time.Time { return now }, seen: map[string]time.Time{}}
	if online, count := p.touch("visitor-a1", true); online != 1 || !count {
		t.Fatalf("first visit: online=%d count=%v", online, count)
	}
	if _, count := p.touch("visitor-a1", true); count {
		t.Error("a repeated new from an online visitor must not count again")
	}
	if online, _ := p.touch("visitor-b2", false); online != 2 {
		t.Errorf("online = %d, want 2", online)
	}
	now = now.Add(6 * time.Minute)
	if online, _ := p.touch("visitor-c3", false); online != 1 {
		t.Errorf("visitors silent for 6 minutes should drop off, online = %d", online)
	}
}
