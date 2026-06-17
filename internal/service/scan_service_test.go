package service

import (
	"testing"

	"github.com/find-assets/scanner/internal/strategy"
)

func TestGet_DayPierceDefaults(t *testing.T) {
	if _, err := strategy.Get("day", "pierce", strategy.Options{}); err != nil {
		t.Fatalf("day:pierce should be available: %v", err)
	}
}

func TestGet_OptionsOverrideRangeVolume(t *testing.T) {
	// Options 为零值时使用形态默认；正值时覆盖。两种情况都应能成功构造。
	if _, err := strategy.Get("week", "pierce", strategy.Options{Range: 1.2, Volume: 30}); err != nil {
		t.Fatalf("week:pierce with options should be available: %v", err)
	}
}

func TestGet_AllCombosAvailable(t *testing.T) {
	for _, pd := range strategy.Periods() {
		for _, pt := range strategy.Patterns() {
			if _, err := strategy.Get(pd.Name, pt.Name, strategy.Options{}); err != nil {
				t.Fatalf("combo %s:%s should be available: %v", pd.Name, pt.Name, err)
			}
		}
	}
}
