// Package trend 实现数字货币「多周期趋势」联合判定（15m + 1h + 4h）。
//
// 判定规则：
//   - 三周期一律取倒数第二根已收盘 K 线（len-2）；末根 len-1 为形成中 K 线，不参与判定
//   - 15m：EMA5/10/30/60/120 中存在长度 ≥3 的方向子序列，且必须包含 EMA60、EMA120
//   - 4h：EMA5/10/30/60/120 严格全排列（多头快线在上，空头对称）
//   - 1h：EMA10/30/60/120 严格方向排列；EMA5 与 EMA10 间距 < MaxEMA5_10GapPct（默认 0.5%）
//   - 1h：EMA30/60/120 相邻间距均 > MinGapPct（默认 1%）
//   - 1h 影线入场：真实触及 EMA30（实体不穿、可贴边），或最低/最高价与 EMA30 差距 < WickNearPct（默认 1%）
//   - 强势：间距均 > StrongMinGapPct（默认 8%）且影线真实触及 EMA30 → Tag 含「强势」且 Alert=true
package trend

import (
	"fmt"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

const (
	DefaultMinGapPct         = 1
	DefaultWickNearPct       = 1
	DefaultStrongMinGapPct   = 8
	DefaultMaxEMA5_10GapPct  = 0.5
	DefaultMinBars           = 250
)

// Direction 趋势方向。
type Direction string

const (
	Bull Direction = "bull" // 多头
	Bear Direction = "bear" // 空头
)

// Options 多周期趋势参数。
type Options struct {
	MinBars          int     // 每周期最少 K 线根数（须含 EMA120），默认 250
	MinGapPct        float64 // 1h EMA30/60/120 间距阈值（百分比），默认 1
	WickNearPct      float64 // 1h 近影线允许差距（百分比），默认 1
	StrongMinGapPct  float64 // 强势间距阈值（百分比），默认 8
	MaxEMA5_10GapPct float64 // 1h EMA5 与 EMA10 最大间距（百分比），默认 0.5
}

// DefaultOptions 返回默认参数。
func DefaultOptions() Options {
	return Options{
		MinBars:          DefaultMinBars,
		MinGapPct:        DefaultMinGapPct,
		WickNearPct:      DefaultWickNearPct,
		StrongMinGapPct:  DefaultStrongMinGapPct,
		MaxEMA5_10GapPct: DefaultMaxEMA5_10GapPct,
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
	if opt.MaxEMA5_10GapPct <= 0 {
		opt.MaxEMA5_10GapPct = DefaultMaxEMA5_10GapPct
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
	strong, ok := MatchDir(*e15, *e4h, *e1h, b1h[len(b1h)-2], bull, opt)
	if !ok {
		return model.Result{}, false
	}
	k := b1h[len(b1h)-2]
	gap60_120 := gapPct(e1h[4], e1h[3])
	gap30_60 := gapPct(e1h[3], e1h[2])
	gap5_10 := gapPct(e1h[0], e1h[1])

	label := "多头"
	if !bull {
		label = "空头"
	}
	tag := fmt.Sprintf("[多周期趋势·%s]", label)
	if strong {
		tag = fmt.Sprintf("[多周期趋势·%s·强势]", label)
	}
	metric := fmt.Sprintf("1h gap60/120=%.2f%% gap30/60=%.2f%% gap5/10=%.2f%% strong=%v",
		gap60_120, gap30_60, gap5_10, strong)
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

// MatchDir 用已算好的三周期 EMA（len-2）与 1h 判定根 K 线判定是否命中；strong 表示原严格条件。
// vals 顺序为 EMA5/10/30/60/120。供示例测试与 Eval 共用，避免规则漂移。
func MatchDir(e15, e4h, e1h [5]float64, k model.Kline, bull bool, opt Options) (strong bool, ok bool) {
	opt = normalizeOptions(opt)
	if !hasAnchorArrangement(e15, bull) || !hasStrictStack(e4h, []int{0, 1, 2, 3, 4}, bull) {
		return false, false
	}
	if !hasStrictStack(e1h, []int{1, 2, 3, 4}, bull) {
		return false, false
	}
	if gapPct(e1h[0], e1h[1]) >= opt.MaxEMA5_10GapPct {
		return false, false
	}
	gap60_120 := gapPct(e1h[4], e1h[3])
	gap30_60 := gapPct(e1h[3], e1h[2])
	if gap60_120 <= opt.MinGapPct || gap30_60 <= opt.MinGapPct {
		return false, false
	}
	if !wickEntry(k, e1h[2], bull, opt.WickNearPct) {
		return false, false
	}
	touch := wickTouches(k, e1h[2], bull)
	strong = touch && gap60_120 > opt.StrongMinGapPct && gap30_60 > opt.StrongMinGapPct
	return strong, true
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

// hasStrictStack 判定 vals 在 idxs 上是否严格同向排列（多头递减、空头递增）。
func hasStrictStack(vals [5]float64, idxs []int, bull bool) bool {
	if len(idxs) < 2 {
		return false
	}
	for j := 1; j < len(idxs); j++ {
		prev, cur := vals[idxs[j-1]], vals[idxs[j]]
		if bull {
			if !(prev > cur) {
				return false
			}
		} else if !(prev < cur) {
			return false
		}
	}
	return true
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
		if hasStrictStack(vals, idxs, bull) {
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
