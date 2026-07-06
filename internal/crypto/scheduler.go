package crypto

import (
	"sort"
	"time"
)

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

// InitSchedule 为每个周期计算首次触发时间。
func InitSchedule(now time.Time, specs []IntervalSpec, delay time.Duration) map[string]time.Time {
	next := make(map[string]time.Time, len(specs))
	for _, spec := range specs {
		next[spec.Name] = NextDelayedRun(now, spec.Duration, delay)
	}
	return next
}

// EarliestNext 返回所有周期中最早的下一次触发时间。
func EarliestNext(next map[string]time.Time) time.Time {
	var earliest time.Time
	for _, t := range next {
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
	}
	return earliest
}

// DueIntervals 返回在 now 时刻已到期的周期名称（按字典序）。
func DueIntervals(now time.Time, next map[string]time.Time) []string {
	due := make([]string, 0, len(next))
	for name, t := range next {
		if !t.After(now) {
			due = append(due, name)
		}
	}
	sort.Strings(due)
	return due
}

// AdvanceInterval 在 now 之后推进指定周期的下一次触发时间。
func AdvanceInterval(now time.Time, spec IntervalSpec, delay time.Duration, next map[string]time.Time) {
	next[spec.Name] = NextDelayedRun(now, spec.Duration, delay)
}

// SpecByName 在列表中查找周期定义。
func SpecByName(specs []IntervalSpec, name string) (IntervalSpec, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}
	}
	return IntervalSpec{}, false
}
