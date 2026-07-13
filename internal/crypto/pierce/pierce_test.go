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

// setSignal 覆盖「判定根」（倒数第二根 len-2）的 OHLC 与量能，并设置其前一根量能。
// 末根（len-1）保持不变，模拟当前周期形成中、不参与判定的 K 线。
func setSignal(bars []model.Kline, o, h, l, c float64, vol, prevVol int64) {
	n := len(bars)
	sig := n - 2
	bars[sig-1].Volume = prevVol
	bars[sig] = model.Kline{Date: bars[sig].Date, Open: o, High: h, Low: l, Close: c, Volume: vol}
}

func TestEvalUpCrossAllStrong(t *testing.T) {
	bars := rampBars(300, 100, 1) // 稳定上行 → 多头排列，EMA120 显著低于快中线簇
	setSignal(bars, 320, 405, 315, 399, 2000, 1000)
	r, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected up-pierce hit")
	}
	if !r.Alert {
		t.Fatalf("crossing all 5 lines should be strong, tag=%s", r.Tag)
	}
	if !strings.Contains(r.Tag, "上穿") || !strings.Contains(r.Tag, "穿5线") || !strings.Contains(r.Tag, "★强势") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalUpCrossFourEma120BelowStrong(t *testing.T) {
	bars := rampBars(300, 100, 1)
	setSignal(bars, 345, 405, 340, 399, 2000, 1000) // 实体 (345,399)：EMA120 在下方未被穿，穿快中线簇 4 根
	r, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected up-pierce hit")
	}
	if !r.Alert {
		t.Fatalf("up-pierce 4 lines with EMA120 below should be strong, tag=%s", r.Tag)
	}
	if !strings.Contains(r.Tag, "穿4线") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalUpNeedsFourLines(t *testing.T) {
	bars := rampBars(300, 100, 1)
	setSignal(bars, 396, 400, 395, 398.5, 2000, 1000) // 实体 (396,398.5) 仅跨 EMA5
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when fewer than 4 lines crossed")
	}
}

func TestEvalUpRequiresVolume(t *testing.T) {
	bars := rampBars(300, 100, 1)
	setSignal(bars, 320, 405, 315, 399, 900, 1000) // 量能未放大
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when up-pierce without volume increase")
	}
}

func TestEvalDownFourEma120AboveStrong(t *testing.T) {
	bars := rampBars(300, 500, -1) // 稳定下行 → 空头排列，EMA120 显著高于快中线簇
	setSignal(bars, 240, 245, 195, 200, 500, 1000) // 阴线，实体 (200,240)：EMA120 在上方未被穿，穿 4 根
	r, ok := Eval(model.Stock{Code: "X"}, bars, Down, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected down-pierce hit regardless of volume")
	}
	if !r.Alert {
		t.Fatalf("down-pierce 4 lines with EMA120 above should be strong, tag=%s", r.Tag)
	}
	if !strings.Contains(r.Tag, "下穿") || !strings.Contains(r.Tag, "穿4线") || !strings.Contains(r.Tag, "★强势") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalWrongArrangementMiss(t *testing.T) {
	bars := rampBars(300, 100, 1)                   // 多头趋势
	setSignal(bars, 399, 405, 315, 320, 2000, 1000) // 阴线穿过均线簇，但趋势为多头 → 下穿方向排列不匹配
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Down, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when arrangement does not match direction")
	}
}

func TestEvalGlueSkips(t *testing.T) {
	bars := flatBars(300, 100)                     // 均线完全重合 → 极度粘合
	setSignal(bars, 95, 115, 90, 110, 2000, 1000)  // 实体虽跨全部均线，但粘合应放弃
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when moving averages are extremely glued")
	}
}

func TestEvalNewCoinDiscarded(t *testing.T) {
	bars := rampBars(150, 100, 1) // n<250，无 EMA120 → 直接舍弃
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
