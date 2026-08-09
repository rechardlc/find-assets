package trend

import (
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

/*
策略示例（判定根 = 各周期 len-2；EMA 顺序 = 5/10/30/60/120）

示例 A · 多头强势
  15m: 148, 146, 142, 138, 125  → 锚定 EMA5>60>120
  4h:  145, 143, 140, 132, 118  → 五线全排列
  1h:  144.2, 144.0, 130, 118, 105
       - EMA10>30>60>120
       - gap(5,10)=0.14% < 0.5%
       - gap(60,120)=11.02% > 8%，gap(30,60)=9.23% > 8%
  1h K: O=146 C=147 H=148 L=128 → Low≤EMA30≤实体（真实触及）
  期望: hit + strong

示例 B · 多头普通（近影线，非强势）
  同 A 的 15m/4h/1h EMA，但 1h K: O=146 C=147 H=148 L=130.5
       - Low=130.5 > EMA30=130，gap≈0.38% < 1%（近影线）
       - 未真实触及 → 非强势
  期望: hit，strong=false

示例 C · 多头 miss · EMA5/10 间距过大
  1h: 146, 144, 130, 118, 105 → gap(5,10)=1.37% ≥ 0.5%
  期望: miss

示例 D · 多头 miss · 4h 未全排列
  4h: 145, 140, 143, 132, 118（EMA10 < EMA30）
  期望: miss

示例 E · 多头 miss · 实体穿 EMA30
  1h K: O=128 C=135 H=136 L=126（实体下沿 128 < EMA30=130）
  期望: miss

示例 F · 空头强势（对称）
  15m: 80, 85, 90, 95, 110
  4h:  80, 85, 90, 100, 115
  1h:  99.8, 100.0, 112, 125, 140
       - gap(5,10)=0.20%，gap(60,120)=10.71%，gap(30,60)=10.40%
  1h K: O=108 C=107 H=114 L=106 → 上影真实触及 EMA30=112
  期望: hit + strong
*/

type exampleCase struct {
	name   string
	e15    [5]float64
	e4h    [5]float64
	e1h    [5]float64
	k      model.Kline
	bull   bool
	wantOK bool
	wantStrong bool
}

func TestStrategyExamples(t *testing.T) {
	opt := DefaultOptions()
	cases := []exampleCase{
		{
			name: "A_多头强势",
			e15:  [5]float64{148, 146, 142, 138, 125},
			e4h:  [5]float64{145, 143, 140, 132, 118},
			e1h:  [5]float64{144.2, 144.0, 130, 118, 105},
			k:    model.Kline{Date: time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC), Open: 146, High: 148, Low: 128, Close: 147},
			bull: true, wantOK: true, wantStrong: true,
		},
		{
			name: "B_多头近影线普通",
			e15:  [5]float64{148, 146, 142, 138, 125},
			e4h:  [5]float64{145, 143, 140, 132, 118},
			e1h:  [5]float64{144.2, 144.0, 130, 118, 105},
			k:    model.Kline{Date: time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC), Open: 146, High: 148, Low: 130.5, Close: 147},
			bull: true, wantOK: true, wantStrong: false,
		},
		{
			name: "C_多头_EMA5_10间距过大",
			e15:  [5]float64{148, 146, 142, 138, 125},
			e4h:  [5]float64{145, 143, 140, 132, 118},
			e1h:  [5]float64{146, 144, 130, 118, 105},
			k:    model.Kline{Open: 146, High: 148, Low: 128, Close: 147},
			bull: true, wantOK: false,
		},
		{
			name: "D_多头_4h未全排列",
			e15:  [5]float64{148, 146, 142, 138, 125},
			e4h:  [5]float64{145, 140, 143, 132, 118},
			e1h:  [5]float64{144.2, 144.0, 130, 118, 105},
			k:    model.Kline{Open: 146, High: 148, Low: 128, Close: 147},
			bull: true, wantOK: false,
		},
		{
			name: "E_多头_实体穿线",
			e15:  [5]float64{148, 146, 142, 138, 125},
			e4h:  [5]float64{145, 143, 140, 132, 118},
			e1h:  [5]float64{144.2, 144.0, 130, 118, 105},
			k:    model.Kline{Open: 128, High: 136, Low: 126, Close: 135},
			bull: true, wantOK: false,
		},
		{
			name: "F_空头强势",
			e15:  [5]float64{80, 85, 90, 95, 110},
			e4h:  [5]float64{80, 85, 90, 100, 115},
			e1h:  [5]float64{99.8, 100.0, 112, 125, 140},
			k:    model.Kline{Date: time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC), Open: 108, High: 114, Low: 106, Close: 107},
			bull: false, wantOK: true, wantStrong: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			strong, ok := MatchDir(tc.e15, tc.e4h, tc.e1h, tc.k, tc.bull, opt)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v (strong=%v)", ok, tc.wantOK, strong)
			}
			if ok && strong != tc.wantStrong {
				t.Fatalf("strong=%v want %v", strong, tc.wantStrong)
			}
			// 校验示例自身数值满足文档声明的关键中间量
			if tc.name == "A_多头强势" || tc.name == "B_多头近影线普通" {
				if g := gapPct(tc.e1h[0], tc.e1h[1]); g >= 0.5 {
					t.Fatalf("example gap5/10=%.3f should be <0.5", g)
				}
				if g := gapPct(tc.e1h[4], tc.e1h[3]); g <= 8 {
					t.Fatalf("example gap60/120=%.3f should be >8", g)
				}
				if g := gapPct(tc.e1h[3], tc.e1h[2]); g <= 8 {
					t.Fatalf("example gap30/60=%.3f should be >8", g)
				}
			}
		})
	}
}

// TestStrategyExamples_EvalRoundTrip 用可复现 K 线构造跑完整 Eval，核对与示例语义一致。
func TestStrategyExamples_EvalRoundTrip(t *testing.T) {
	t.Run("构造多头强势_对齐示例A语义", func(t *testing.T) {
		bars := riseThenFlatBars(300, 100, 1.01, 20)
		forceWickBull(bars)
		r, ok := Eval(model.Stock{Code: "EX-A"}, bars, bars, bars, DefaultOptions())
		if !ok {
			t.Fatal("expected Eval hit for rise-then-flat + real wick")
		}
		if !r.Alert || r.Tag != "[多周期趋势·多头·强势]" {
			t.Fatalf("got tag=%q alert=%v", r.Tag, r.Alert)
		}
		if gapPct(r.Snapshot.EMA5, r.Snapshot.EMA10) >= DefaultMaxEMA5_10GapPct {
			t.Fatalf("EMA5/10 gap too wide: %.3f", gapPct(r.Snapshot.EMA5, r.Snapshot.EMA10))
		}
	})
	t.Run("构造多头近影线_对齐示例B语义", func(t *testing.T) {
		bars := riseThenFlatBars(300, 100, 1.01, 20)
		forceNearWickBull(bars)
		r, ok := Eval(model.Stock{Code: "EX-B"}, bars, bars, bars, DefaultOptions())
		if !ok {
			t.Fatal("expected Eval hit for near wick")
		}
		if r.Alert || r.Tag != "[多周期趋势·多头]" {
			t.Fatalf("got tag=%q alert=%v", r.Tag, r.Alert)
		}
	})
	t.Run("构造空头强势_对齐示例F语义", func(t *testing.T) {
		bars := riseThenFlatBars(300, 10000, 0.99, 20)
		forceWickBear(bars)
		r, ok := Eval(model.Stock{Code: "EX-F"}, bars, bars, bars, DefaultOptions())
		if !ok || !r.Alert || r.Tag != "[多周期趋势·空头·强势]" {
			t.Fatalf("got ok=%v tag=%q alert=%v", ok, r.Tag, r.Alert)
		}
	})
}
