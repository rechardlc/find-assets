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
