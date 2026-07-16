package reversal

import (
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

func TestEval_RejectsInsufficientBars(t *testing.T) {
	bars := makeFlatKlines(100)
	_, ok := Eval(model.Stock{Code: "TESTUSDT"}, bars, Oversold, DefaultOptions("15m"))
	if ok {
		t.Fatal("expected insufficient bars to reject")
	}
}

func TestEval_IgnoresVolume(t *testing.T) {
	bars := makeFlatKlines(300)
	// 末段放量不应单独导致命中；平坦 K 线本身也不应命中，此处只验证不会因 panic 崩溃。
	bars[len(bars)-1].Volume = bars[len(bars)-2].Volume * 100
	bars[len(bars)-2].Volume = bars[len(bars)-3].Volume * 100
	_, _ = Eval(model.Stock{Code: "TESTUSDT"}, bars, Oversold, DefaultOptions("15m"))
	_, _ = Eval(model.Stock{Code: "TESTUSDT"}, bars, Overbought, DefaultOptions("15m"))
}

func TestEval_CrossOffsetUsesSecondBar(t *testing.T) {
	opt := DefaultOptions("15m")
	if opt.CrossOffset != 2 {
		t.Fatalf("expected cross offset 2, got %d", opt.CrossOffset)
	}
}

func TestGoldenCrossAtMatchesDeadCrossMirror(t *testing.T) {
	fast := []float64{1, 2, 3, 2, 1, 2, 3}
	slow := []float64{2, 2, 2, 2, 2, 2, 2}
	if !indicator.DeadCrossAt(fast, slow, 4) {
		t.Fatal("expected dead cross at index 4")
	}

	fastUp := []float64{3, 2, 1, 2, 3, 4, 5}
	slowUp := []float64{2, 2, 2, 2, 2, 2, 2}
	if !indicator.GoldenCrossAt(fastUp, slowUp, 4) {
		t.Fatal("expected golden cross at index 4")
	}
}

func TestHasStrongShadow_Overbought(t *testing.T) {
	// 五根 K 线，末根上影线远大于实体且最高价为窗口最高。
	bars := []model.Kline{
		{Open: 10, Close: 10.2, High: 10.5, Low: 9.9},
		{Open: 10.2, Close: 10.4, High: 10.6, Low: 10.1},
		{Open: 10.4, Close: 10.5, High: 10.7, Low: 10.3},
		{Open: 10.5, Close: 10.6, High: 10.8, Low: 10.4},
		{Open: 10.6, Close: 10.7, High: 11.6, Low: 10.5}, // 上影线 0.9 > 实体 0.1，最高价 11.6 为窗口最高
	}
	if !hasStrongShadow(bars, len(bars)-1, Overbought) {
		t.Fatal("expected strong overbought shadow")
	}
	// 窗口最高价的那根实体大于影线，其余长影线根都不是窗口最高 → 不算强势。
	notStrong := []model.Kline{
		{Open: 10, Close: 10.2, High: 10.9, Low: 9.9},   // 上影 0.7 > 实体 0.2，但非窗口最高
		{Open: 10.2, Close: 10.4, High: 10.6, Low: 10.1}, // 短影
		{Open: 10.4, Close: 10.5, High: 10.7, Low: 10.3}, // 短影
		{Open: 10.5, Close: 10.6, High: 10.8, Low: 10.4}, // 短影
		{Open: 10.6, Close: 11.6, High: 11.65, Low: 10.5}, // 窗口最高，但实体 1.0 > 上影 0.05
	}
	if hasStrongShadow(notStrong, len(notStrong)-1, Overbought) {
		t.Fatal("expected non-strong when the window high has a short shadow")
	}
}

func TestHasStrongShadow_Oversold(t *testing.T) {
	// 五根 K 线，末根下影线远大于实体且最低价为窗口最低。
	bars := []model.Kline{
		{Open: 10, Close: 9.8, High: 10.1, Low: 9.7},
		{Open: 9.8, Close: 9.6, High: 9.9, Low: 9.5},
		{Open: 9.6, Close: 9.5, High: 9.7, Low: 9.4},
		{Open: 9.5, Close: 9.4, High: 9.6, Low: 9.3},
		{Open: 9.4, Close: 9.3, High: 9.5, Low: 8.4}, // 下影线 0.9 > 实体 0.1，最低价 8.4 为窗口最低
	}
	if !hasStrongShadow(bars, len(bars)-1, Oversold) {
		t.Fatal("expected strong oversold shadow")
	}
	// 上影线大于实体但方向不匹配，超跌不应命中。
	flat := makeFlatKlines(5)
	if hasStrongShadow(flat, len(flat)-1, Oversold) {
		t.Fatal("expected flat klines to be non-strong")
	}
}

func TestHasStrongShadow_InsufficientBars(t *testing.T) {
	bars := makeFlatKlines(4)
	if hasStrongShadow(bars, len(bars)-1, Overbought) {
		t.Fatal("expected false when fewer than 5 bars available")
	}
}

func makeFlatKlines(n int) []model.Kline {
	out := make([]model.Kline, n)
	start := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	for i := range out {
		out[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * 15 * time.Minute),
			Open:   1,
			Close:  1,
			High:   1,
			Low:    1,
			Volume: 1000,
		}
	}
	return out
}
