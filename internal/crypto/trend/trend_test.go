package trend

import (
	"strings"
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

func TestGapPct(t *testing.T) {
	g := gapPct(110, 100)
	if g < 9.0 || g > 9.2 {
		t.Fatalf("gapPct=%.4f", g)
	}
}

func TestHasStrictStackBull(t *testing.T) {
	vals := [5]float64{120, 115, 110, 105, 100}
	if !hasStrictStack(vals, []int{0, 1, 2, 3, 4}, true) {
		t.Fatal("expected full bull stack")
	}
	if !hasStrictStack(vals, []int{1, 2, 3, 4}, true) {
		t.Fatal("expected 10/30/60/120 bull stack")
	}
	vals2 := [5]float64{120, 110, 115, 105, 100}
	if hasStrictStack(vals2, []int{0, 1, 2, 3, 4}, true) {
		t.Fatal("expected miss when EMA10 < EMA30")
	}
}

func TestHasAnchorArrangementBull(t *testing.T) {
	vals := [5]float64{120, 115, 90, 110, 100}
	if !hasAnchorArrangement(vals, true) {
		t.Fatal("expected bull anchor arrangement")
	}
	vals2 := [5]float64{120, 50, 40, 110, 100}
	if !hasAnchorArrangement(vals2, true) {
		t.Fatal("expected bull with EMA5+60+120")
	}
	vals3 := [5]float64{130, 125, 120, 100, 110}
	if hasAnchorArrangement(vals3, true) {
		t.Fatal("expected miss when EMA60 < EMA120")
	}
}

func TestHasAnchorArrangementBear(t *testing.T) {
	vals := [5]float64{80, 85, 130, 90, 100}
	if !hasAnchorArrangement(vals, false) {
		t.Fatal("expected bear anchor arrangement")
	}
}

func TestWickTouchesBull(t *testing.T) {
	k := model.Kline{Open: 110, Close: 112, High: 115, Low: 100}
	if !wickTouches(k, 105, true) {
		t.Fatal("expected lower wick touch")
	}
	k2 := model.Kline{Open: 108, Close: 112, High: 115, Low: 100}
	if wickTouches(k2, 109, true) {
		t.Fatal("expected miss when body crosses EMA30")
	}
	k3 := model.Kline{Open: 105, Close: 110, High: 112, Low: 100}
	if !wickTouches(k3, 105, true) {
		t.Fatal("expected hit when body edge equals EMA30")
	}
}

func TestWickTouchesBear(t *testing.T) {
	k := model.Kline{Open: 90, Close: 88, High: 105, Low: 85}
	if !wickTouches(k, 100, false) {
		t.Fatal("expected upper wick touch")
	}
	k2 := model.Kline{Open: 95, Close: 88, High: 105, Low: 85}
	if wickTouches(k2, 92, false) {
		t.Fatal("expected miss when body crosses EMA30")
	}
}

func TestWickEntryNearBull(t *testing.T) {
	ema30 := 100.0
	k := model.Kline{Open: 102, Close: 103, High: 104, Low: 100.5}
	if !wickEntry(k, ema30, true, 1) {
		t.Fatal("expected near wick entry")
	}
	if wickTouches(k, ema30, true) {
		t.Fatal("near case should not count as real touch")
	}
	kFar := model.Kline{Open: 104, Close: 105, High: 106, Low: 102.5}
	if wickEntry(kFar, ema30, true, 1) {
		t.Fatal("expected miss when Low is >1% above EMA30")
	}
}

func TestWickEntryNearBear(t *testing.T) {
	ema30 := 100.0
	k := model.Kline{Open: 97, Close: 96, High: 99.5, Low: 95}
	if !wickEntry(k, ema30, false, 1) {
		t.Fatal("expected near upper wick entry")
	}
	kFar := model.Kline{Open: 95, Close: 94, High: 97.5, Low: 93}
	if wickEntry(kFar, ema30, false, 1) {
		t.Fatal("expected miss when High is >1% below EMA30")
	}
}

func TestEvalBullStrongHit(t *testing.T) {
	bars15 := riseThenFlatBars(300, 100, 1.01, 20)
	bars1h := riseThenFlatBars(300, 100, 1.01, 20)
	bars4h := riseThenFlatBars(300, 100, 1.01, 20)
	forceWickBull(bars1h)
	r, ok := Eval(model.Stock{Code: "BTCUSDT", Name: "BTC"}, bars15, bars1h, bars4h, DefaultOptions())
	if !ok {
		t.Fatal("expected bull hit")
	}
	if !strings.Contains(r.Tag, "多头·强势") {
		t.Fatalf("tag=%q want 强势", r.Tag)
	}
	if !r.Alert {
		t.Fatal("expected Alert for strong")
	}
	if r.Snapshot.EMA30 == 0 || r.Snapshot.EMA60 == 0 || r.Snapshot.EMA120 == 0 {
		t.Fatal("expected EMA snapshot")
	}
	if gapPct(r.Snapshot.EMA5, r.Snapshot.EMA10) >= DefaultMaxEMA5_10GapPct {
		t.Fatalf("expected EMA5/10 gap < 0.5, got %.3f", gapPct(r.Snapshot.EMA5, r.Snapshot.EMA10))
	}
}

func TestEvalBullNearWickNormalHit(t *testing.T) {
	bars15 := riseThenFlatBars(300, 100, 1.01, 20)
	bars1h := riseThenFlatBars(300, 100, 1.01, 20)
	bars4h := riseThenFlatBars(300, 100, 1.01, 20)
	forceNearWickBull(bars1h)
	r, ok := Eval(model.Stock{Code: "BTCUSDT", Name: "BTC"}, bars15, bars1h, bars4h, DefaultOptions())
	if !ok {
		t.Fatal("expected bull hit with near wick")
	}
	if strings.Contains(r.Tag, "强势") {
		t.Fatalf("near wick should not be strong, tag=%q", r.Tag)
	}
	if r.Alert {
		t.Fatal("expected Alert=false for near-only")
	}
	if !strings.Contains(r.Tag, "多头") {
		t.Fatalf("tag=%q", r.Tag)
	}
}

func TestEvalEMA5_10GapTooWideMiss(t *testing.T) {
	// 纯指数上涨：EMA5/EMA10 间距通常 > 0.5%
	bars := expBars(300, 100, 1.01)
	forceWickBull(bars)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, bars, bars, DefaultOptions()); ok {
		t.Fatal("expected miss when EMA5/10 gap too wide")
	}
}

func TestEvalGapTooSmallMiss(t *testing.T) {
	bars := flatBars(300, 100)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, bars, bars, DefaultOptions()); ok {
		t.Fatal("expected miss on flat")
	}
}

func TestEvalBodyCrossMiss(t *testing.T) {
	bars15 := riseThenFlatBars(300, 100, 1.01, 20)
	bars1h := riseThenFlatBars(300, 100, 1.01, 20)
	bars4h := riseThenFlatBars(300, 100, 1.01, 20)
	forceBodyCrossBull(bars1h)
	if _, ok := Eval(model.Stock{Code: "X"}, bars15, bars1h, bars4h, DefaultOptions()); ok {
		t.Fatal("expected miss when body crosses")
	}
}

func TestEvalBearStrongHit(t *testing.T) {
	bars15 := riseThenFlatBars(300, 10000, 0.99, 20)
	bars1h := riseThenFlatBars(300, 10000, 0.99, 20)
	bars4h := riseThenFlatBars(300, 10000, 0.99, 20)
	forceWickBear(bars1h)
	r, ok := Eval(model.Stock{Code: "X"}, bars15, bars1h, bars4h, DefaultOptions())
	if !ok || !strings.Contains(r.Tag, "空头·强势") {
		t.Fatalf("expected bear strong hit, got ok=%v tag=%q", ok, r.Tag)
	}
	if !r.Alert {
		t.Fatal("expected Alert for strong bear")
	}
}

func TestEvalInsufficientBars(t *testing.T) {
	bars := riseThenFlatBars(100, 100, 1.01, 20)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, bars, bars, DefaultOptions()); ok {
		t.Fatal("expected miss when bars < MinBars")
	}
}

func TestDefaultOptionsGap(t *testing.T) {
	opt := DefaultOptions()
	if opt.MinGapPct != 1 || opt.WickNearPct != 1 || opt.StrongMinGapPct != 8 || opt.MaxEMA5_10GapPct != 0.5 {
		t.Fatalf("unexpected defaults: %+v", opt)
	}
}

func flatBars(n int, price float64) []model.Kline {
	bars := make([]model.Kline, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		bars[i] = model.Kline{
			Date: start.Add(time.Duration(i) * time.Hour),
			Open: price, High: price, Low: price, Close: price, Volume: 1000,
		}
	}
	return bars
}

// expBars 纯指数序列（EMA5/10 通常拉开，用于负例）。
func expBars(n int, base, growth float64) []model.Kline {
	return riseThenFlatBars(n, base, growth, 0)
}

// riseThenFlatBars 先趋势再横盘，便于 EMA5≈EMA10 且中长线仍拉开。
func riseThenFlatBars(n int, base, growth float64, flat int) []model.Kline {
	bars := make([]model.Kline, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	price := base
	rise := n - flat
	if rise < 1 {
		rise = 1
	}
	for i := range bars {
		bars[i] = model.Kline{
			Date: start.Add(time.Duration(i) * time.Hour),
			Open: price, High: price, Low: price, Close: price, Volume: 1000,
		}
		if i < rise-1 {
			price *= growth
		}
	}
	return bars
}

func ema30AtSignal(bars []model.Kline) float64 {
	closes := model.Closes(bars)
	e30 := indicator.EMA(closes, 30)
	return e30[len(bars)-2]
}

// forceWickBull 改写 len-2：下影刺穿 ema30，实体完全在上方；保留 Close 以免重算 EMA 漂移。
func forceWickBull(bars []model.Kline) {
	sig := len(bars) - 2
	e30 := ema30AtSignal(bars)
	c := bars[sig].Close
	bars[sig].Open = c
	bars[sig].Close = c
	bars[sig].High = c
	bars[sig].Low = e30 - 3
}

// forceNearWickBull 改写 len-2：Low 略高于 ema30（约 0.5%），不触及；实体在上方。
func forceNearWickBull(bars []model.Kline) {
	sig := len(bars) - 2
	e30 := ema30AtSignal(bars)
	c := bars[sig].Close
	bars[sig].Open = c
	bars[sig].Close = c
	bars[sig].High = c
	bars[sig].Low = e30 * 1.005
}

// forceWickBear 改写 len-2：上影刺穿 ema30，实体完全在下方；保留 Close。
func forceWickBear(bars []model.Kline) {
	sig := len(bars) - 2
	e30 := ema30AtSignal(bars)
	c := bars[sig].Close
	bars[sig].Open = c
	bars[sig].Close = c
	bars[sig].Low = c
	bars[sig].High = e30 + 3
}

// forceBodyCrossBull 实体下沿低于 ema30；保留 Close 以免 EMA 漂移后穿线失效。
func forceBodyCrossBull(bars []model.Kline) {
	sig := len(bars) - 2
	e30 := ema30AtSignal(bars)
	c := bars[sig].Close
	bars[sig].Open = e30 * 0.99
	bars[sig].Close = c
	bars[sig].High = c
	bars[sig].Low = e30 * 0.98
}
