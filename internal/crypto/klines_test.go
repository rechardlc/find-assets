package crypto

import (
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

func TestDropFormingBarRemovesCurrentPeriod(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 7, 13, 11, 0, 20, 0, loc) // 11:00:20 扫描
	bars := []model.Kline{
		{Date: time.Date(2026, 7, 13, 9, 0, 0, 0, loc), Close: 99},
		{Date: time.Date(2026, 7, 13, 10, 0, 0, 0, loc), Close: 100}, // 10:00–11:00 已收盘
		{Date: time.Date(2026, 7, 13, 11, 0, 0, 0, loc), Close: 101}, // 11:00 刚开，未收盘
	}

	got := DropFormingBar(bars, time.Hour, now)
	if len(got) != 2 {
		t.Fatalf("expected 2 bars after drop, got %d", len(got))
	}
	if got[len(got)-1].Close != 100 {
		t.Fatalf("expected last closed bar close=100, got %v", got[len(got)-1])
	}
}

func TestDropFormingBarKeepsWhenLatestClosed(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 7, 13, 11, 5, 0, 0, loc) // 非边界时刻手动扫描
	bars := []model.Kline{
		{Date: time.Date(2026, 7, 13, 9, 0, 0, 0, loc), Close: 99},
		{Date: time.Date(2026, 7, 13, 10, 0, 0, 0, loc), Close: 100},
	}

	got := DropFormingBar(bars, time.Hour, now)
	if len(got) != 2 {
		t.Fatalf("expected unchanged 2 bars, got %d", len(got))
	}
}
