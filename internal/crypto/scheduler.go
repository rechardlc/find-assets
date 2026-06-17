package crypto

import "time"

func NextDelayedRun(now time.Time, interval, delay time.Duration) time.Time {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	base := now.Truncate(interval)
	next := base.Add(interval).Add(delay)
	if !next.After(now) {
		next = next.Add(interval)
	}
	return next
}
