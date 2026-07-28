package amplitude

import (
	"strings"
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

func flatBars(n int, price float64) []model.Kline {
	bars := make([]model.Kline, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		bars[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * 4 * time.Hour),
			Open:   price,
			High:   price,
			Low:    price,
			Close:  price,
			Volume: 1000,
		}
	}
	return bars
}

// setSignal 覆盖判定根（上一根已收盘 K 线 len-2）的 OHLC；末根（len-1）保持不变，
// 模拟当前周期形成中、不参与判定的 K 线。
func setSignal(bars []model.Kline, o, h, l, c float64) {
	sig := len(bars) - 2
	bars[sig] = model.Kline{Date: bars[sig].Date, Open: o, High: h, Low: l, Close: c, Volume: 2000}
}

func TestEvalUpHit(t *testing.T) {
	bars := flatBars(10, 100)
	setSignal(bars, 101, 110, 100, 109) // 振幅 10%
	r, ok := Eval(model.Stock{Code: "X"}, bars, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected hit when amplitude exceeds 9%")
	}
	if r.Alert {
		t.Fatalf("10%% should not be strong under 2x threshold, tag=%s", r.Tag)
	}
	if !strings.Contains(r.Tag, "4小时") || !strings.Contains(r.Tag, "情绪涨") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
	if got := r.Snapshot.Amplitude; got < 9.99 || got > 10.01 {
		t.Fatalf("amplitude = %v, want ~10", got)
	}
	if r.Snapshot.High != 110 || r.Snapshot.Low != 100 {
		t.Fatalf("snapshot high/low = %v/%v, want 110/100", r.Snapshot.High, r.Snapshot.Low)
	}
}

func TestEvalDownHit(t *testing.T) {
	bars := flatBars(10, 100)
	setSignal(bars, 109, 110, 100, 101)
	r, ok := Eval(model.Stock{Code: "X"}, bars, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected hit for bearish bar with 10% amplitude")
	}
	if !strings.Contains(r.Tag, "情绪跌") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalFlatDirection(t *testing.T) {
	bars := flatBars(10, 100)
	setSignal(bars, 105, 115, 100, 105) // 开收相等，上下扫针
	r, ok := Eval(model.Stock{Code: "X"}, bars, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected hit regardless of direction")
	}
	if !strings.Contains(r.Tag, "情绪分歧") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalBelowThresholdMiss(t *testing.T) {
	bars := flatBars(10, 100)
	setSignal(bars, 100, 108, 100, 107) // 振幅 8% < 9%
	if _, ok := Eval(model.Stock{Code: "X"}, bars, DefaultOptions("4h")); ok {
		t.Fatal("expected miss when amplitude below threshold")
	}
}

func TestEvalStrongAtDoubleThreshold(t *testing.T) {
	bars := flatBars(10, 100)
	setSignal(bars, 100, 118, 100, 117) // 振幅 18% = 2x 阈值
	r, ok := Eval(model.Stock{Code: "X"}, bars, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected hit")
	}
	if !r.Alert || !strings.Contains(r.Tag, "★强势") {
		t.Fatalf("18%% should be strong, tag=%s alert=%v", r.Tag, r.Alert)
	}
}

func TestEvalCustomThreshold(t *testing.T) {
	bars := flatBars(10, 100)
	setSignal(bars, 100, 106, 100, 105) // 振幅 6%
	opt := DefaultOptions("15m")
	opt.MinPct = 5
	if _, ok := Eval(model.Stock{Code: "X"}, bars, opt); !ok {
		t.Fatal("expected hit when threshold lowered to 5%")
	}
	opt.MinPct = 7
	if _, ok := Eval(model.Stock{Code: "X"}, bars, opt); ok {
		t.Fatal("expected miss when threshold raised to 7%")
	}
}

// 末根为形成中的 K 线，其巨大振幅不应触发命中。
func TestEvalIgnoresFormingBar(t *testing.T) {
	bars := flatBars(10, 100)
	last := len(bars) - 1
	bars[last] = model.Kline{Date: bars[last].Date, Open: 100, High: 150, Low: 100, Close: 149}
	if _, ok := Eval(model.Stock{Code: "X"}, bars, DefaultOptions("4h")); ok {
		t.Fatal("expected miss: forming bar must not be evaluated")
	}
}

func TestEvalNewCoinStillEvaluated(t *testing.T) {
	bars := flatBars(3, 100) // 仅 3 根，新上线合约
	setSignal(bars, 100, 120, 100, 119)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, DefaultOptions("4h")); !ok {
		t.Fatal("expected hit: amplitude strategy does not filter new listings")
	}
}

func TestEvalTooFewBarsMiss(t *testing.T) {
	bars := flatBars(1, 100)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, DefaultOptions("4h")); ok {
		t.Fatal("expected miss when there is no closed previous bar")
	}
}

func TestPctInvalidData(t *testing.T) {
	if _, ok := Pct(model.Kline{High: 10, Low: 0}); ok {
		t.Fatal("expected false for non-positive low")
	}
	if _, ok := Pct(model.Kline{High: 5, Low: 10}); ok {
		t.Fatal("expected false when high < low")
	}
}

func TestMinRequiredBarsFloor(t *testing.T) {
	if got := MinRequiredBars(Options{MinBars: 0}); got != minBarsFloor {
		t.Fatalf("MinRequiredBars = %d, want %d", got, minBarsFloor)
	}
	if got := MinRequiredBars(Options{MinBars: 60}); got != 60 {
		t.Fatalf("MinRequiredBars = %d, want 60", got)
	}
}
