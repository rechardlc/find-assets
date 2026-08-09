// Package trend 实现数字货币「多周期趋势」联合判定（15m + 1h + 4h）。
//
// 判定规则：
//   - 三周期一律取倒数第二根已收盘 K 线（len-2）；末根 len-1 为形成中 K 线，不参与判定
//   - 15m、4h：EMA5/10/30/60/120 中存在长度 ≥3 的方向子序列，且必须包含 EMA60、EMA120
//   - 1h：EMA30/60/120 严格方向排列，且相邻间距均 > MinGapPct（默认 1%）
//   - 1h 影线入场：真实触及 EMA30（实体不穿、可贴边），或最低/最高价与 EMA30 差距 < WickNearPct（默认 1%）
//   - 强势：间距均 > StrongMinGapPct（默认 8%）且影线真实触及 EMA30 → Tag 含「强势」且 Alert=true
package trend

import (
	"fmt"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

const (
	DefaultMinGapPct       = 1
	DefaultWickNearPct     = 1
	DefaultStrongMinGapPct = 8
	DefaultMinBars         = 250
)

// Direction 趋势方向。
type Direction string

const (
	Bull Direction = "bull" // 多头
	Bear Direction = "bear" // 空头
)

// Options 多周期趋势参数。
type Options struct {
	MinBars         int     // 每周期最少 K 线根数（须含 EMA120），默认 250
	MinGapPct       float64 // 1h EMA 间距阈值（百分比），默认 1
	WickNearPct     float64 // 1h 近影线允许差距（百分比），默认 1
	StrongMinGapPct float64 // 强势间距阈值（百分比），默认 8
}

// DefaultOptions 返回默认参数。
func DefaultOptions() Options {
	return Options{
		MinBars:         DefaultMinBars,
		MinGapPct:       DefaultMinGapPct,
		WickNearPct:     DefaultWickNearPct,
		StrongMinGapPct: DefaultStrongMinGapPct,
	}
}

// MinRequiredBars 返回参与判定所需的最少 K 线根数。
func MinRequiredBars(opt Options) int {
	if opt.MinBars <= 0 {
		return DefaultMinBars
	}
	return opt.MinBars
}

// Eval 联合判定 15m/1h/4h；多头优先，至多命中一侧。
func Eval(stock model.Stock, bars15m, bars1h, bars4h []model.Kline, opt Options) (model.Result, bool) {
	opt = normalizeOptions(opt)
	minN := opt.MinBars
	if len(bars15m) < minN || len(bars1h) < minN || len(bars4h) < minN {
		return model.Result{}, false
	}

	for _, dir := range []Direction{Bull, Bear} {
		if r, ok := evalDir(stock, bars15m, bars1h, bars4h, dir, opt); ok {
			return r, true
		}
	}
	return model.Result{}, false
}

func normalizeOptions(opt Options) Options {
	if opt.MinBars <= 0 {
		opt.MinBars = DefaultMinBars
	}
	if opt.MinGapPct <= 0 {
		opt.MinGapPct = DefaultMinGapPct
	}
	if opt.WickNearPct <= 0 {
		opt.WickNearPct = DefaultWickNearPct
	}
	if opt.StrongMinGapPct <= 0 {
		opt.StrongMinGapPct = DefaultStrongMinGapPct
	}
	return opt
}

func evalDir(stock model.Stock, b15, b1h, b4h []model.Kline, dir Direction, opt Options) (model.Result, bool) {
	bull := dir == Bull
	e15 := emasAt(b15, len(b15)-2)
	e4h := emasAt(b4h, len(b4h)-2)
	e1h := emasAt(b1h, len(b1h)-2)
	if e15 == nil || e4h == nil || e1h == nil {
		return model.Result{}, false
	}
	if !hasAnchorArrangement(*e15, bull) || !hasAnchorArrangement(*e4h, bull) {
		return model.Result{}, false
	}
	// 1h: indices 2/3/4 = EMA30/60/120
	if bull {
		if !(e1h[2] > e1h[3] && e1h[3] > e1h[4]) {
			return model.Result{}, false
		}
	} else if !(e1h[2] < e1h[3] && e1h[3] < e1h[4]) {
		return model.Result{}, false
	}
	gap60_120 := gapPct(e1h[4], e1h[3])
	gap30_60 := gapPct(e1h[3], e1h[2])
	if gap60_120 <= opt.MinGapPct || gap30_60 <= opt.MinGapPct {
		return model.Result{}, false
	}
	k := b1h[len(b1h)-2]
	if !wickEntry(k, e1h[2], bull, opt.WickNearPct) {
		return model.Result{}, false
	}

	touch := wickTouches(k, e1h[2], bull)
	strong := touch && gap60_120 > opt.StrongMinGapPct && gap30_60 > opt.StrongMinGapPct

	label := "多头"
	if !bull {
		label = "空头"
	}
	tag := fmt.Sprintf("[多周期趋势·%s]", label)
	if strong {
		tag = fmt.Sprintf("[多周期趋势·%s·强势]", label)
	}
	metric := fmt.Sprintf("1h gap60/120=%.2f%% gap30/60=%.2f%% strong=%v", gap60_120, gap30_60, strong)
	return model.Result{
		Code:   stock.Code,
		Name:   stock.Name,
		Tag:    tag,
		Metric: metric,
		Alert:  strong,
		Snapshot: model.Snapshot{
			Date:   k.Date.Format("2006-01-02 15:04"),
			Close:  k.Close,
			High:   k.High,
			Low:    k.Low,
			EMA5:   e1h[0],
			EMA10:  e1h[1],
			EMA30:  e1h[2],
			EMA60:  e1h[3],
			EMA120: e1h[4],
			Bars:   len(b1h),
		},
	}, true
}

// emasAt 返回 bars[idx] 处 EMA5/10/30/60/120。
func emasAt(bars []model.Kline, idx int) *[5]float64 {
	if idx < 0 || idx >= len(bars) {
		return nil
	}
	closes := model.Closes(bars)
	e5 := indicator.EMA(closes, 5)
	e10 := indicator.EMA(closes, 10)
	e30 := indicator.EMA(closes, 30)
	e60 := indicator.EMA(closes, 60)
	e120 := indicator.EMA(closes, 120)
	return &[5]float64{e5[idx], e10[idx], e30[idx], e60[idx], e120[idx]}
}

func gapPct(a, b float64) float64 {
	hi := a
	if b > hi {
		hi = b
	}
	if hi == 0 {
		return 0
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff / hi * 100
}

// hasAnchorArrangement 判定 vals[5,10,30,60,120] 是否存在长度 ≥3 的严格方向子序列，
// 且必须包含 EMA60（下标 3）与 EMA120（下标 4）。
func hasAnchorArrangement(vals [5]float64, bull bool) bool {
	for mask := 1; mask < 8; mask++ {
		idxs := make([]int, 0, 5)
		for i := 0; i < 3; i++ {
			if mask&(1<<i) != 0 {
				idxs = append(idxs, i)
			}
		}
		idxs = append(idxs, 3, 4)
		ok := true
		for j := 1; j < len(idxs); j++ {
			prev, cur := vals[idxs[j-1]], vals[idxs[j]]
			if bull {
				if !(prev > cur) {
					ok = false
					break
				}
			} else if !(prev < cur) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// wickTouches 判定影线触及 ema30：多头下影、空头上影；实体不穿、可贴边。
func wickTouches(k model.Kline, ema30 float64, bull bool) bool {
	bodyLo, bodyHi := k.Open, k.Close
	if bodyLo > bodyHi {
		bodyLo, bodyHi = bodyHi, bodyLo
	}
	if bull {
		return k.Low <= ema30 && ema30 <= bodyLo
	}
	return bodyHi <= ema30 && ema30 <= k.High
}

// wickEntry 1h 入场：真实触及，或极值与 EMA30 差距 < nearPct 且实体不穿（可贴边）。
func wickEntry(k model.Kline, ema30 float64, bull bool, nearPct float64) bool {
	if wickTouches(k, ema30, bull) {
		return true
	}
	bodyLo, bodyHi := k.Open, k.Close
	if bodyLo > bodyHi {
		bodyLo, bodyHi = bodyHi, bodyLo
	}
	if bull {
		if ema30 > bodyLo {
			return false
		}
		return gapPct(k.Low, ema30) < nearPct
	}
	if bodyHi > ema30 {
		return false
	}
	return gapPct(k.High, ema30) < nearPct
}
