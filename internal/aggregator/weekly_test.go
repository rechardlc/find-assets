package aggregator

import (
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

func mkDay(t *testing.T, ymd string, o, c, h, l float64) model.Kline {
	t.Helper()
	d, err := time.Parse("2006-01-02", ymd)
	if err != nil {
		t.Fatal(err)
	}
	return model.Kline{Date: d, Open: o, Close: c, High: h, Low: l, Volume: 100}
}

func TestToWeekly_OHLC(t *testing.T) {
	// 2024-01-01 是周一，到 2024-01-05 周五，共 5 个交易日 → 1 根周线
	daily := []model.Kline{
		mkDay(t, "2024-01-01", 10, 11, 12, 9),
		mkDay(t, "2024-01-02", 11, 13, 14, 10),
		mkDay(t, "2024-01-03", 13, 12, 15, 11),
		mkDay(t, "2024-01-04", 12, 14, 16, 8),
		mkDay(t, "2024-01-05", 14, 15, 15, 13),
	}
	wk := ToWeekly(daily)
	if len(wk) != 1 {
		t.Fatalf("expected 1 weekly bar, got %d", len(wk))
	}
	w := wk[0]
	if w.Open != 10 {
		t.Errorf("open = %v, want 10", w.Open)
	}
	if w.Close != 15 {
		t.Errorf("close = %v, want 15", w.Close)
	}
	if w.High != 16 {
		t.Errorf("high = %v, want 16", w.High)
	}
	if w.Low != 8 {
		t.Errorf("low = %v, want 8", w.Low)
	}
	if w.Volume != 500 {
		t.Errorf("volume = %v, want 500", w.Volume)
	}
}

func TestToWeekly_CrossWeeks(t *testing.T) {
	// 跨两周（第一周仅一天，第二周完整）
	daily := []model.Kline{
		mkDay(t, "2024-01-05", 10, 10, 10, 10),  // 周一不是，是周五 → 单独一周
		mkDay(t, "2024-01-08", 11, 11, 11, 11),  // 周一
		mkDay(t, "2024-01-09", 12, 12, 12, 12),
	}
	wk := ToWeekly(daily)
	if len(wk) != 2 {
		t.Fatalf("expected 2 weekly bars, got %d", len(wk))
	}
	if wk[0].Close != 10 || wk[1].Close != 12 {
		t.Errorf("unexpected weekly closes: %v, %v", wk[0].Close, wk[1].Close)
	}
}

func TestToWeekly_Empty(t *testing.T) {
	if out := ToWeekly(nil); out != nil {
		t.Fatalf("expected nil, got %v", out)
	}
}
