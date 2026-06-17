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
