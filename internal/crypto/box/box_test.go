package box

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

// rampBars 构造单调价格序列：相邻两根相差 |step|（默认 1%），远超 0.6% 带宽阈值，
// 因此基准段自身凑不出箱体，测试里只有显式植入的价位才会形成箱体。
func rampBars(n int, base, step float64) []model.Kline {
	bars := make([]model.Kline, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := base
	for i := range bars {
		bars[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * time.Hour),
			Open:   p,
			High:   p * 1.001,
			Low:    p,
			Close:  p,
			Volume: 1000,
		}
		p *= 1 + step
	}
	return bars
}

// bottomBase 基准低点远高于测试箱体价位（100 附近），确保只有植入的低点参与判定。
func bottomBase(n int) []model.Kline { return rampBars(n, 200, 0.01) }

// topBase 基准高点远低于测试箱体价位（120 附近）。
func topBase(n int) []model.Kline { return rampBars(n, 50, -0.01) }

func setLowAt(bars []model.Kline, idx int, low float64) {
	bars[idx].Low = low
	bars[idx].Open = low * 1.002
	bars[idx].Close = low * 1.003
	bars[idx].High = low * 1.005
}

func setHighAt(bars []model.Kline, idx int, high float64) {
	bars[idx].High = high
	bars[idx].Open = high * 0.998
	bars[idx].Close = high * 0.997
	bars[idx].Low = high * 0.995
}

func TestEvalBottomHit(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 10, 100.0)
	setLowAt(bars, 18, 100.4)
	setLowAt(bars, 28, 100.2) // 末根已收盘 K 线，必须是箱体成员

	r, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected hit: three lows within 0.6% and last closed bar on the box")
	}
	if r.Snapshot.Touches != 3 {
		t.Fatalf("touches = %d, want 3", r.Snapshot.Touches)
	}
	if r.Snapshot.Low != 100.0 || r.Snapshot.High != 100.4 {
		t.Fatalf("box range = %v~%v, want 100~100.4", r.Snapshot.Low, r.Snapshot.High)
	}
	if got := r.Snapshot.Range; math.Abs(got-0.4) > 0.01 {
		t.Fatalf("box width = %v%%, want ~0.4%%", got)
	}
	if r.Alert {
		t.Fatalf("3 touches should not be strong, tag=%s", r.Tag)
	}
	for _, want := range []string{"1小时", "底部箱体", "触及3次"} {
		if !strings.Contains(r.Tag, want) {
			t.Fatalf("tag %q missing %q", r.Tag, want)
		}
	}
	if !strings.Contains(r.Metric, "跨 19 根") || !strings.Contains(r.Metric, "距下沿") {
		t.Fatalf("unexpected metric: %s", r.Metric)
	}
}

// 用户原始例子：连续三根踩在同一水平线上。
func TestEvalBottomHitOnConsecutiveBars(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 26, 100.0)
	setLowAt(bars, 27, 100.5)
	setLowAt(bars, 28, 100.3)

	r, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected hit for three consecutive bars sharing a support level")
	}
	if !strings.Contains(r.Metric, "跨 3 根") {
		t.Fatalf("unexpected metric: %s", r.Metric)
	}
}

// 箱体区间内出现更低的低点：新下沿把原成员挤出带宽，剩余触及次数不足。
func TestEvalBottomMissOnBreakdown(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 10, 100.0)
	setLowAt(bars, 14, 99.0) // 跌破下沿
	setLowAt(bars, 18, 100.4)
	setLowAt(bars, 28, 100.2)

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h")); ok {
		t.Fatal("expected miss: support was broken inside the box")
	}
}

func TestEvalBottomMissWhenWidthExceeded(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 10, 100.0)
	setLowAt(bars, 18, 100.8) // 与 100.0 相差 0.8% > 0.6%
	setLowAt(bars, 28, 100.2)

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h")); ok {
		t.Fatal("expected miss: no three lows fit inside 0.6%")
	}
}

// 箱体已经走完、末根脱离箱体：信号过期，不应命中。
func TestEvalBottomRequiresLastBarOnBox(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 10, 100.0)
	setLowAt(bars, 14, 100.4)
	setLowAt(bars, 18, 100.2)

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h")); ok {
		t.Fatal("expected miss: last closed bar left the box")
	}
}

func TestEvalTopHit(t *testing.T) {
	bars := topBase(30)
	setHighAt(bars, 10, 120.0)
	setHighAt(bars, 18, 119.5)
	setHighAt(bars, 28, 119.8)

	r, ok := Eval(model.Stock{Code: "X"}, bars, Top, DefaultOptions("4h"))
	if !ok {
		t.Fatal("expected hit: three highs within 0.6% and last closed bar on the box")
	}
	if r.Snapshot.Touches != 3 {
		t.Fatalf("touches = %d, want 3", r.Snapshot.Touches)
	}
	if r.Snapshot.Low != 119.5 || r.Snapshot.High != 120.0 {
		t.Fatalf("box range = %v~%v, want 119.5~120", r.Snapshot.Low, r.Snapshot.High)
	}
	for _, want := range []string{"4小时", "顶部箱体", "触及3次"} {
		if !strings.Contains(r.Tag, want) {
			t.Fatalf("tag %q missing %q", r.Tag, want)
		}
	}
	if !strings.Contains(r.Metric, "距上沿") {
		t.Fatalf("unexpected metric: %s", r.Metric)
	}
}

func TestEvalTopMissOnBreakout(t *testing.T) {
	bars := topBase(30)
	setHighAt(bars, 10, 120.0)
	setHighAt(bars, 14, 121.5) // 上破箱体
	setHighAt(bars, 18, 119.5)
	setHighAt(bars, 28, 119.8)

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Top, DefaultOptions("4h")); ok {
		t.Fatal("expected miss: resistance was broken inside the box")
	}
}

// 末根（len-1）为形成中的 K 线，其极端价格不参与判定。
func TestEvalIgnoresFormingBar(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 10, 100.0)
	setLowAt(bars, 18, 100.4)
	setLowAt(bars, 28, 100.2)
	setLowAt(bars, 29, 50) // 形成中的末根暴跌

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h")); !ok {
		t.Fatal("expected hit: forming bar must not affect the box")
	}
}

func TestEvalAlertOnManyTouches(t *testing.T) {
	bars := bottomBase(30)
	for idx, low := range map[int]float64{10: 100.0, 14: 100.2, 18: 100.3, 22: 100.5, 28: 100.1} {
		setLowAt(bars, idx, low)
	}

	r, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected hit")
	}
	if r.Snapshot.Touches != 5 {
		t.Fatalf("touches = %d, want 5", r.Snapshot.Touches)
	}
	if !r.Alert || !strings.Contains(r.Tag, "★强势") {
		t.Fatalf("5 touches should be strong, tag=%s alert=%v", r.Tag, r.Alert)
	}
}

func TestEvalCustomWidthAndTouches(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 10, 100.0)
	setLowAt(bars, 18, 100.8)
	setLowAt(bars, 28, 100.2)

	opt := DefaultOptions("1h")
	opt.MaxWidthPct = 1
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, opt); !ok {
		t.Fatal("expected hit when width relaxed to 1%")
	}

	opt = DefaultOptions("1h")
	opt.MinTouches = 4
	setLowAt(bars, 18, 100.4)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, opt); ok {
		t.Fatal("expected miss when 4 touches required but only 3 present")
	}
}

// 超出回看窗口的低点不计入触及次数。
func TestEvalRespectsLookbackWindow(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 10, 100.0)
	setLowAt(bars, 18, 100.4)
	setLowAt(bars, 28, 100.2)

	opt := DefaultOptions("1h")
	opt.Lookback = 5 // 窗口仅 24~28
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, opt); ok {
		t.Fatal("expected miss: earlier touches fall outside the lookback window")
	}
}

func TestEvalInvalidPriceMiss(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 10, 100.0)
	setLowAt(bars, 18, 100.4)
	setLowAt(bars, 28, 100.2)
	bars[12].Low = 0 // 数据异常

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h")); ok {
		t.Fatal("expected miss on non-positive low")
	}
}

func TestEvalTooFewBarsMiss(t *testing.T) {
	bars := bottomBase(3)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h")); ok {
		t.Fatal("expected miss: fewer bars than the minimum touches plus forming bar")
	}
}

func TestEvalUnknownDirectionMiss(t *testing.T) {
	bars := bottomBase(30)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Direction("side"), DefaultOptions("1h")); ok {
		t.Fatal("expected miss for unknown direction")
	}
}

func TestMinRequiredBars(t *testing.T) {
	if got, want := MinRequiredBars(DefaultOptions("1h")), DefaultTouches+1; got != want {
		t.Fatalf("MinRequiredBars = %d, want %d", got, want)
	}
	if got := MinRequiredBars(Options{MinTouches: 5}); got != 6 {
		t.Fatalf("MinRequiredBars = %d, want 6", got)
	}
}

func TestDefaultOptions(t *testing.T) {
	opt := DefaultOptions("4h")
	if opt.MaxWidthPct != DefaultPct || opt.Lookback != DefaultLookback || opt.MinTouches != DefaultTouches {
		t.Fatalf("unexpected defaults: %+v", opt)
	}
	if opt.AlertTouches != DefaultAlertTouches || opt.Interval != "4h" {
		t.Fatalf("unexpected defaults: %+v", opt)
	}
}
