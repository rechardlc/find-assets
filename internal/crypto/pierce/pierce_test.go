package pierce

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
			Date:   start.Add(time.Duration(i) * time.Hour),
			Open:   price,
			High:   price,
			Low:    price,
			Close:  price,
			Volume: 1000,
		}
	}
	return bars
}

func rampBars(n int, base, slope float64) []model.Kline {
	bars := make([]model.Kline, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		price := base + slope*float64(i)
		bars[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * time.Hour),
			Open:   price,
			High:   price,
			Low:    price,
			Close:  price,
			Volume: 1000,
		}
	}
	return bars
}

// stepBars 前 split 根价格为 a，其余为 b，用于拉开 EMA120 与快中线簇。
func stepBars(n, split int, a, b float64) []model.Kline {
	bars := make([]model.Kline, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		price := a
		if i >= split {
			price = b
		}
		bars[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * time.Hour),
			Open:   price,
			High:   price,
			Low:    price,
			Close:  price,
			Volume: 1000,
		}
	}
	return bars
}

// setSignal 覆盖「判定根」（倒数第二根 len-2）的 OHLC 与量能，并设置其前一根量能。
// 末根（len-1）保持不变，模拟当前周期形成中、不参与判定的 K 线。
func setSignal(bars []model.Kline, o, h, l, c float64, vol, prevVol int64) {
	n := len(bars)
	sig := n - 2
	bars[sig-1].Volume = prevVol
	bars[sig] = model.Kline{Date: bars[sig].Date, Open: o, High: h, Low: l, Close: c, Volume: vol}
}

func TestEvalUpCrossAllStrong(t *testing.T) {
	bars := flatBars(300, 100)
	setSignal(bars, 99.90, 100.20, 99.80, 100.10, 2000, 1000)
	r, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected up-pierce hit when glued and piercing all 5 lines")
	}
	if !r.Alert {
		t.Fatalf("crossing all 5 lines should be strong, tag=%s", r.Tag)
	}
	if !strings.Contains(r.Tag, "上穿") || !strings.Contains(r.Tag, "穿5线") || !strings.Contains(r.Tag, "★强势") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalUpCrossFourEma120BelowStrong(t *testing.T) {
	bars := stepBars(300, 180, 80, 100)
	setSignal(bars, 99.4, 100.6, 99.2, 100.5, 2000, 1000)
	r, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected up-pierce hit")
	}
	if !r.Alert {
		t.Fatalf("up-pierce 4 lines with EMA120 below should be strong, tag=%s ema120=%.4f", r.Tag, r.Snapshot.EMA120)
	}
	if !strings.Contains(r.Tag, "穿4线") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalUpNeedsFourLines(t *testing.T) {
	bars := flatBars(300, 100)
	setSignal(bars, 100.02, 100.03, 100.01, 100.03, 2000, 1000) // 实体在均线上方，未跨过
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when fewer than 4 lines crossed")
	}
}

func TestEvalUpRequiresVolume(t *testing.T) {
	bars := flatBars(300, 100)
	setSignal(bars, 99.90, 100.20, 99.80, 100.10, 900, 1000)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when up-pierce without volume increase")
	}
}

func TestEvalDownFourEma120AboveStrong(t *testing.T) {
	bars := stepBars(300, 180, 120, 100)
	setSignal(bars, 100.6, 100.8, 99.2, 99.4, 500, 1000)
	r, ok := Eval(model.Stock{Code: "X"}, bars, Down, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected down-pierce hit regardless of volume")
	}
	if !r.Alert {
		t.Fatalf("down-pierce 4 lines with EMA120 above should be strong, tag=%s ema120=%.4f", r.Tag, r.Snapshot.EMA120)
	}
	if !strings.Contains(r.Tag, "下穿") || !strings.Contains(r.Tag, "穿4线") || !strings.Contains(r.Tag, "★强势") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalSpreadMissesGlue(t *testing.T) {
	bars := rampBars(300, 100, 1)
	setSignal(bars, 320, 405, 315, 399, 2000, 1000)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when EMA ribbon is not glued")
	}
}

func TestGlueThreshold(t *testing.T) {
	if got := glueThreshold("15m"); got != GluePct15m {
		t.Fatalf("15m glue = %g, want %g", got, GluePct15m)
	}
	if got := glueThreshold("1h"); got != GluePct1h {
		t.Fatalf("1h glue = %g, want %g", got, GluePct1h)
	}
	if got := glueThreshold("4h"); got != GluePct4h {
		t.Fatalf("4h glue = %g, want %g", got, GluePct4h)
	}
	if got := glueThreshold("unknown"); got != 0 {
		t.Fatalf("unknown glue = %g, want 0", got)
	}
}

func TestEvalGlueRequiredOn15m(t *testing.T) {
	bars := flatBars(300, 100)
	setSignal(bars, 99.90, 100.20, 99.80, 100.10, 2000, 1000)
	r, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("15m"))
	if !ok {
		t.Fatal("expected 15m up-pierce hit when glued")
	}
	if !strings.Contains(r.Tag, "15分钟") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalNewCoinDiscarded(t *testing.T) {
	bars := rampBars(150, 100, 1)
	setSignal(bars, 190, 210, 185, 205, 2000, 1000)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss for coins without EMA120 (new coin discarded)")
	}
}

func TestEvalSkipsWhenTooFewBars(t *testing.T) {
	bars := rampBars(100, 100, 1)
	setSignal(bars, 150, 160, 145, 155, 2000, 1000)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when fewer than 250 bars")
	}
}

func TestHasGluedPiercedFour(t *testing.T) {
	// 快中 4 线带宽 0.03%，EMA120 在外侧且未被穿。
	lines := [5]float64{100.00, 100.01, 100.02, 100.03, 110}
	if !hasGluedPiercedFour(lines, 99.9, 100.1, GluePct15m) {
		t.Fatal("expected glued 4 of 5 pierced at 15m threshold")
	}
	// 穿的是拉开的线，粘合的 4 根没被穿。
	if hasGluedPiercedFour(lines, 109, 111, GluePct15m) {
		t.Fatal("expected miss when pierced lines are not the glued set")
	}
	// 任意 4 根带宽约 0.09%，过不了 15m/1h，过得了 4h。
	wide := [5]float64{100.00, 100.03, 100.06, 100.09, 100.12}
	if hasGluedPiercedFour(wide, 99, 101, GluePct15m) {
		t.Fatal("expected 15m miss when 4-line diameter is 0.09%")
	}
	if hasGluedPiercedFour(wide, 99, 101, GluePct1h) {
		t.Fatal("expected 1h miss when 4-line diameter is 0.09%")
	}
	if !hasGluedPiercedFour(wide, 99, 101, GluePct4h) {
		t.Fatal("expected 4h hit when 4-line diameter is 0.09%")
	}
}
