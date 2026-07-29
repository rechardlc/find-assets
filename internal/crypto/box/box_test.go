package box

import (
	"fmt"
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
	if !strings.Contains(r.Metric, "振幅") || r.Snapshot.Amplitude <= DefaultMinAmpPct {
		t.Fatalf("metric/snapshot must carry the span amplitude: metric=%s amp=%v", r.Metric, r.Snapshot.Amplitude)
	}
}

// flatBars 构造贴着同一价位的死水序列：低点全为 base，高点为 base*(1+amp%)，
// 箱体本身成立（触及次数、跨度、带宽都够），只用来验证跨度内振幅门槛。
func flatBars(n int, base, ampPct float64) []model.Kline {
	bars := make([]model.Kline, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	high := base + base*ampPct/100
	for i := range bars {
		bars[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * time.Hour),
			Open:   base,
			High:   high,
			Low:    base,
			Close:  base,
			Volume: 1000,
		}
	}
	return bars
}

// 振幅门槛边界：4.9% 不命中、5% 命中（与带宽/触及次数同为 >= 口径）。
func TestEvalAmplitudeBoundary(t *testing.T) {
	cases := []struct {
		name   string
		ampPct float64
		want   bool
	}{
		{"below threshold", 4.9, false},
		{"exactly at threshold", 5, true},
		{"above threshold", 8, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bars := flatBars(30, 100, tc.ampPct)
			for _, dir := range []Direction{Bottom, Top} {
				r, ok := Eval(model.Stock{Code: "X"}, bars, dir, DefaultOptions("1h"))
				if ok != tc.want {
					t.Fatalf("dir=%s hit = %v, want %v (amplitude %.2f%%)", dir, ok, tc.want, tc.ampPct)
				}
				if ok && math.Abs(r.Snapshot.Amplitude-tc.ampPct) > 0.01 {
					t.Fatalf("dir=%s amplitude = %v, want ~%v", dir, r.Snapshot.Amplitude, tc.ampPct)
				}
			}
		})
	}
}

// 振幅取箱体内所有 K 线的最高 High 与最低 Low，而非首末两根的价格：
// 首末两根只差 1%，但中间某根冲高 8%，仍应命中且振幅记为 8%。
func TestEvalAmplitudeUsesBoxExtremesNotEndBars(t *testing.T) {
	bars := flatBars(30, 100, 1)
	bars[20].High = 108 // 箱体正中间的冲高，首末两根都碰不到

	r, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected hit: the highest high inside the box is 8% above the lowest low")
	}
	if math.Abs(r.Snapshot.Amplitude-8) > 0.01 {
		t.Fatalf("amplitude = %v, want ~8 (highest high vs lowest low inside the box)", r.Snapshot.Amplitude)
	}
}

func TestEvalCustomMinAmpPct(t *testing.T) {
	bars := flatBars(30, 100, 3)

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h")); ok {
		t.Fatal("expected miss at default amplitude threshold of 5%")
	}
	opt := DefaultOptions("1h")
	opt.MinAmpPct = 2
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, opt); !ok {
		t.Fatal("expected hit when amplitude threshold lowered to 2%")
	}
}

// 紧凑候选振幅不足时，应回退到跨度更宽、振幅达标的候选。
func TestEvalFallsBackToWiderZoneOnAmplitude(t *testing.T) {
	bars := flatBars(30, 100, 1) // 12 之后是死水，单靠这段振幅只有 1%
	bars[12].High = 106          // 起点退到 12 才凑出 6% 振幅
	for i := 0; i < 12; i++ {
		bars[i].Low = 99 // 更低的下沿会把末根挤出带宽，候选无法再往前扩
	}

	r, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected hit: the wider zone reaches the amplitude threshold")
	}
	if !strings.Contains(r.Metric, "跨 17 根") {
		t.Fatalf("span must cover the amplitude source bar, got: %s", r.Metric)
	}
	if math.Abs(r.Snapshot.Amplitude-6) > 0.01 {
		t.Fatalf("amplitude = %v, want ~6", r.Snapshot.Amplitude)
	}
}

// 连续三根挤在一起：触及次数够，但首末触及之间没有中间 K 线，不算箱体震荡。
func TestEvalBottomMissOnConsecutiveBars(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 26, 100.0)
	setLowAt(bars, 27, 100.5)
	setLowAt(bars, 28, 100.3)

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("4h")); ok {
		t.Fatal("expected miss: three consecutive bars leave no gap between first and last touch")
	}
}

// 跨度门槛边界：中间 5 根不命中、正好 6 根命中。
func TestEvalBottomGapBoundary(t *testing.T) {
	cases := []struct {
		name  string
		first int
		want  bool
	}{
		{"middle 5 bars", 22, false},
		{"middle 6 bars", 21, true},
		{"middle 7 bars", 20, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bars := bottomBase(30)
			setLowAt(bars, tc.first, 100.0)
			setLowAt(bars, tc.first+1, 100.4) // 中间成员，不影响首末间隔
			setLowAt(bars, 28, 100.2)

			r, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h"))
			if ok != tc.want {
				t.Fatalf("hit = %v, want %v (middle bars = %d)", ok, tc.want, 28-tc.first-1)
			}
			if ok && !strings.Contains(r.Metric, fmt.Sprintf("跨 %d 根", 28-tc.first+1)) {
				t.Fatalf("unexpected span in metric: %s", r.Metric)
			}
		})
	}
}

// 更紧凑的候选跨度不足时，应回退到跨度足够的候选，且跨度按真实首次触及计算。
func TestEvalBottomFallsBackToWiderZone(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 12, 100.0) // 真实首次触及
	setLowAt(bars, 26, 100.4) // 与末根相邻，单独成不了箱体
	setLowAt(bars, 28, 100.2)

	r, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, DefaultOptions("1h"))
	if !ok {
		t.Fatal("expected hit: the wider zone satisfies the gap threshold")
	}
	if !strings.Contains(r.Metric, "跨 17 根") {
		t.Fatalf("span must be measured from the real first touch, got: %s", r.Metric)
	}
}

func TestEvalTopMissOnConsecutiveBars(t *testing.T) {
	bars := topBase(30)
	setHighAt(bars, 26, 120.0)
	setHighAt(bars, 27, 119.5)
	setHighAt(bars, 28, 119.8)

	if _, ok := Eval(model.Stock{Code: "X"}, bars, Top, DefaultOptions("4h")); ok {
		t.Fatal("expected miss: gap threshold applies to top boxes as well")
	}
}

func TestEvalCustomMinGap(t *testing.T) {
	bars := bottomBase(30)
	setLowAt(bars, 24, 100.0) // 中间 3 根
	setLowAt(bars, 26, 100.4)
	setLowAt(bars, 28, 100.2)

	opt := DefaultOptions("1h")
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, opt); ok {
		t.Fatal("expected miss at default gap of 6")
	}
	opt.MinGap = 3
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, opt); !ok {
		t.Fatal("expected hit when gap threshold lowered to 3")
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
	opt.MinGap = 1 // 排除跨度门槛的干扰，只验证窗口截断
	opt.Lookback = 12
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, opt); ok {
		t.Fatal("expected miss: window 17~28 keeps only two touches")
	}
	opt.Lookback = 24
	if _, ok := Eval(model.Stock{Code: "X"}, bars, Bottom, opt); !ok {
		t.Fatal("expected hit once the window covers all three touches")
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
	// 默认下跨度需求（间隔 6 + 首末 2 根）大于触及次数需求。
	if got, want := MinRequiredBars(DefaultOptions("1h")), DefaultMinGap+3; got != want {
		t.Fatalf("MinRequiredBars = %d, want %d", got, want)
	}
	// 触及次数需求更大时以它为准。
	if got := MinRequiredBars(Options{MinTouches: 12, MinGap: 1}); got != 13 {
		t.Fatalf("MinRequiredBars = %d, want 13", got)
	}
}

func TestDefaultOptions(t *testing.T) {
	opt := DefaultOptions("4h")
	if opt.MaxWidthPct != DefaultPct || opt.Lookback != DefaultLookback || opt.MinTouches != DefaultTouches {
		t.Fatalf("unexpected defaults: %+v", opt)
	}
	if opt.MinGap != DefaultMinGap || opt.AlertTouches != DefaultAlertTouches || opt.Interval != "4h" {
		t.Fatalf("unexpected defaults: %+v", opt)
	}
	if opt.MinAmpPct != DefaultMinAmpPct {
		t.Fatalf("unexpected amplitude default: %+v", opt)
	}
}
