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

// setLast 覆盖最后一根 K 线的 OHLC 与量能（Close 会参与 EMA 计算）。
func setLast(bars []model.Kline, o, h, l, c float64, vol, prevVol int64) {
	n := len(bars)
	bars[n-2].Volume = prevVol
	bars[n-1] = model.Kline{Date: bars[n-1].Date, Open: o, High: h, Low: l, Close: c, Volume: vol}
}

func TestEvalUpCrossesLines(t *testing.T) {
	bars := flatBars(300, 100)
	setLast(bars, 95, 115, 90, 110, 2000, 1000) // 阳线，实体 (95,110) 跨过全部 5 条均线
	r, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected up-pierce hit")
	}
	if r.Alert {
		t.Fatalf("did not expect alert, tag=%s", r.Tag)
	}
	if !strings.Contains(r.Tag, "上穿") || !strings.Contains(r.Tag, "老币") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalUpNeedsFourLines(t *testing.T) {
	bars := flatBars(300, 100)
	setLast(bars, 101, 115, 100, 110, 2000, 1000) // 实体 (101,110) 仅跨过 EMA5/EMA10
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when fewer than 4 lines crossed")
	}
}

func TestEvalUpRequiresVolume(t *testing.T) {
	bars := flatBars(300, 100)
	setLast(bars, 95, 115, 90, 110, 900, 1000) // 量能未放大
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when up-pierce without volume increase")
	}
}

func TestEvalDownIgnoresVolume(t *testing.T) {
	bars := flatBars(300, 100)
	setLast(bars, 101, 102, 88, 90, 500, 1000) // 阴线，实体 (90,101) 跨过全部均线，量能缩小也应命中
	r, ok := Eval(model.Stock{Code: "X"}, bars, Down, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected down-pierce hit regardless of volume")
	}
	if r.Alert {
		t.Fatal("down-pierce should never be alert")
	}
	if !strings.Contains(r.Tag, "下穿") {
		t.Fatalf("unexpected tag: %s", r.Tag)
	}
}

func TestEvalUpSpecialAlert(t *testing.T) {
	bars := rampBars(300, 100, 1) // 稳定上行，EMA120 远低于价格
	setLast(bars, 360, 400, 355, 399, 2000, 1000)
	r, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected up-pierce hit")
	}
	if !r.Alert {
		t.Fatalf("expected special alert, tag=%s", r.Tag)
	}
	if !strings.Contains(r.Tag, "★") {
		t.Fatalf("alert tag should contain marker, got %s", r.Tag)
	}
}

func TestEvalNewCoinUsesFourLines(t *testing.T) {
	bars := flatBars(150, 100) // 120<=n<250 → 新币，仅 EMA5/10/30/60
	setLast(bars, 95, 115, 90, 110, 2000, 1000)
	r, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected new-coin up-pierce hit")
	}
	if !strings.Contains(r.Tag, "新币") {
		t.Fatalf("expected 新币 tag, got %s", r.Tag)
	}
	if r.Alert {
		t.Fatal("new coin has no EMA120, should not alert")
	}
}

func TestEvalSkipsWhenTooFewBars(t *testing.T) {
	bars := flatBars(100, 100)
	setLast(bars, 95, 115, 90, 110, 2000, 1000)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Up, DefaultOptions("1h")); ok {
		t.Fatal("expected miss when fewer than 120 bars")
	}
}
