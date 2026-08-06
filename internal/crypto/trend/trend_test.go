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

func TestEvalBullHit(t *testing.T) {
	bars15 := expBars(300, 100, 1.01)
	bars1h := expBars(300, 100, 1.01)
	bars4h := expBars(300, 100, 1.01)
	forceWickBull(bars1h)
	r, ok := Eval(model.Stock{Code: "BTCUSDT", Name: "BTC"}, bars15, bars1h, bars4h, DefaultOptions())
	if !ok {
		t.Fatal("expected bull hit")
	}
	if !strings.Contains(r.Tag, "多头") {
		t.Fatalf("tag=%q", r.Tag)
	}
	if r.Snapshot.EMA30 == 0 || r.Snapshot.EMA60 == 0 || r.Snapshot.EMA120 == 0 {
		t.Fatal("expected EMA snapshot")
	}
}

func TestEvalGapTooSmallMiss(t *testing.T) {
	bars := flatBars(300, 100)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, bars, bars, DefaultOptions()); ok {
		t.Fatal("expected miss on flat")
	}
}

func TestEvalBodyCrossMiss(t *testing.T) {
	bars15 := expBars(300, 100, 1.01)
	bars1h := expBars(300, 100, 1.01)
	bars4h := expBars(300, 100, 1.01)
	forceBodyCrossBull(bars1h)
	if _, ok := Eval(model.Stock{Code: "X"}, bars15, bars1h, bars4h, DefaultOptions()); ok {
		t.Fatal("expected miss when body crosses")
	}
}

func TestEvalBearHit(t *testing.T) {
	bars15 := expBars(300, 10000, 0.99)
	bars1h := expBars(300, 10000, 0.99)
	bars4h := expBars(300, 10000, 0.99)
	forceWickBear(bars1h)
	r, ok := Eval(model.Stock{Code: "X"}, bars15, bars1h, bars4h, DefaultOptions())
	if !ok || !strings.Contains(r.Tag, "空头") {
		t.Fatalf("expected bear hit, got ok=%v tag=%q", ok, r.Tag)
	}
}

func TestEvalInsufficientBars(t *testing.T) {
	bars := expBars(100, 100, 1.01)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, bars, bars, DefaultOptions()); ok {
		t.Fatal("expected miss when bars < MinBars")
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

// expBars 指数增长/衰减序列，便于拉开 EMA30/60/120 间距超过 8%。
func expBars(n int, base, growth float64) []model.Kline {
	bars := make([]model.Kline, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	price := base
	for i := range bars {
		bars[i] = model.Kline{
			Date: start.Add(time.Duration(i) * time.Hour),
			Open: price, High: price, Low: price, Close: price, Volume: 1000,
		}
		price *= growth
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

// forceBodyCrossBull 实体下沿低于 ema30，破坏「仅影线」条件；改 Close 会微移 EMA，但足以穿实体。
func forceBodyCrossBull(bars []model.Kline) {
	sig := len(bars) - 2
	e30 := ema30AtSignal(bars)
	bars[sig].Open = e30 - 2
	bars[sig].Close = e30 + 4
	bars[sig].High = e30 + 6
	bars[sig].Low = e30 - 5
}
