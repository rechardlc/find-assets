package crypto

import (
	"testing"
	"time"
)

func TestNextDelayedRunAlignsToNextIntervalBoundary(t *testing.T) {
	now := time.Date(2026, 6, 17, 9, 7, 3, 0, time.Local)
	got := NextDelayedRun(now, 15*time.Minute, 20*time.Second)
	want := time.Date(2026, 6, 17, 9, 15, 20, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestNextDelayedRunMovesPastCurrentDelayedBoundary(t *testing.T) {
	now := time.Date(2026, 6, 17, 9, 15, 21, 0, time.Local)
	got := NextDelayedRun(now, 15*time.Minute, 20*time.Second)
	want := time.Date(2026, 6, 17, 9, 30, 20, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestDueIntervalsReturnsAllPastDue(t *testing.T) {
	now := time.Date(2026, 6, 17, 8, 0, 20, 0, time.Local)
	next := map[string]time.Time{
		"15m": time.Date(2026, 6, 17, 8, 0, 20, 0, time.Local),
		"1h":  time.Date(2026, 6, 17, 8, 0, 20, 0, time.Local),
		"4h":  time.Date(2026, 6, 17, 12, 0, 20, 0, time.Local),
	}
	due := DueIntervals(now, next)
	if len(due) != 2 || due[0] != "15m" || due[1] != "1h" {
		t.Fatalf("unexpected due intervals: %#v", due)
	}
}

func TestInitScheduleAndAdvance(t *testing.T) {
	now := time.Date(2026, 6, 17, 9, 7, 0, 0, time.Local)
	specs := []IntervalSpec{{Name: "15m", Duration: 15 * time.Minute}}
	delay := 20 * time.Second
	next := InitSchedule(now, specs, delay)
	want := time.Date(2026, 6, 17, 9, 15, 20, 0, time.Local)
	if !next["15m"].Equal(want) {
		t.Fatalf("want %s, got %s", want, next["15m"])
	}
	runAt := time.Date(2026, 6, 17, 9, 15, 21, 0, time.Local)
	AdvanceInterval(runAt, specs[0], delay, next)
	wantNext := time.Date(2026, 6, 17, 9, 30, 20, 0, time.Local)
	if !next["15m"].Equal(wantNext) {
		t.Fatalf("want %s, got %s", wantNext, next["15m"])
	}
}
