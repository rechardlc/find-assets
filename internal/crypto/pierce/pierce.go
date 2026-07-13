// Package pierce 实现数字货币专用的「一箭穿心」形态判定，与 A 股 internal/strategy 完全独立。
//
// 判定规则（仅作用于有 EMA120 的老币，新币直接舍弃）：
//   - 穿心/粘合判定对象为**倒数第二根已收盘 K 线**（len-2）；序列末尾（len-1）为当前周期在形成中的 K 线
//   - 排列判定对象为**最新根**（len-1）：先穿心、下一根仍维持方向排列才算命中
//   - 双向：上穿（阳线自下而上跨过均线）/ 下穿（阴线自上而下跨过均线）
//   - 「实体跨越」：被穿的均线严格夹在开盘价与收盘价之间
//   - 需穿 EMA5/10/30/60/120 中任意 4 根，不足 4 根放弃
//   - 上穿要求放量（当根成交量 > 前一根），下穿不要求
//   - 方向匹配的均线排列过滤：以 len-1 为结尾的最近 5 根 K 线中至少 4 根，EMA5/10/30/60/120 任意 4 根呈方向排列
//     （上穿需多头排列、下穿需空头排列）
//   - 均线极度粘合（len-2 相邻均线间距全部低于阈值：1h < 0.1%、4h < 0.4%）时放弃
//   - 强势标记（Alert）：穿满 5 根，或 穿 4 根且 EMA120 位于强势一侧
//     （上穿 → EMA120 在实体下方；下穿 → EMA120 在实体上方）
package pierce

import (
	"fmt"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

// Direction 一箭穿心方向。
type Direction string

const (
	Up   Direction = "up"   // 上穿：阳线自下而上跨过均线
	Down Direction = "down" // 下穿：阴线自上而下跨过均线
)

// Options 数字货币一箭穿心参数（与 A 股 pierce 独立）。
type Options struct {
	MinBars  int    // 参与判定所需最少 K 线（须含 EMA120），默认 250
	MinCross int    // 至少跨越几根均线，默认 4
	Interval string // 周期标识，用于 Tag 与粘合阈值，例如 "1h"
}

func DefaultOptions(interval string) Options {
	return Options{MinBars: 250, MinCross: 4, Interval: interval}
}

// MinRequiredBars 返回参与判定所需的最少 K 线根数。
func MinRequiredBars(opt Options) int {
	if opt.MinBars <= 0 {
		return 250
	}
	return opt.MinBars
}

// Eval 判定一箭穿心：穿心/粘合在倒数第二根已收盘 K 线（len-2），排列在最新根（len-1）确认。
func Eval(stock model.Stock, bars []model.Kline, dir Direction, opt Options) (model.Result, bool) {
	if opt.MinBars <= 0 {
		opt.MinBars = 250
	}
	if opt.MinCross <= 0 {
		opt.MinCross = 4
	}

	n := len(bars)
	// 只保留老币：不足 MinBars（无 EMA120）直接舍弃。
	if n < opt.MinBars {
		return model.Result{}, false
	}

	closes := model.Closes(bars)
	ema5 := indicator.EMA(closes, 5)
	ema10 := indicator.EMA(closes, 10)
	ema30 := indicator.EMA(closes, 30)
	ema60 := indicator.EMA(closes, 60)
	ema120 := indicator.EMA(closes, 120)

	// 穿心判定取倒数第二根已收盘 K 线（len-2）；排列/粘合判定取最新根（len-1）。
	last := n - 2
	arrLast := n - 1
	// 需要 last-1（前一根量能）与 arrLast-4（5 根排列窗口）。
	if last-1 < 0 || arrLast-4 < 0 {
		return model.Result{}, false
	}
	k := bars[last]
	prev := bars[last-1]

	// 实体上下沿：上穿取 [开盘价, 收盘价]（阳线），下穿取 [收盘价, 开盘价]（阴线）。
	var lo, hi float64
	switch dir {
	case Up:
		if !(k.Close > k.Open) {
			return model.Result{}, false
		}
		lo, hi = k.Open, k.Close
	case Down:
		if !(k.Close < k.Open) {
			return model.Result{}, false
		}
		lo, hi = k.Close, k.Open
	default:
		return model.Result{}, false
	}

	// 统计被实体严格跨越的均线。
	lines := [5]float64{ema5[last], ema10[last], ema30[last], ema60[last], ema120[last]}
	crossed := 0
	for _, v := range lines {
		if lo < v && v < hi {
			crossed++
		}
	}
	if crossed < opt.MinCross {
		return model.Result{}, false
	}

	// 上穿要求放量（当根成交量 > 前一根）；下穿不要求。
	volInc := 0.0
	if dir == Up {
		if prev.Volume <= 0 || k.Volume <= prev.Volume {
			return model.Result{}, false
		}
		volInc = (float64(k.Volume) - float64(prev.Volume)) / float64(prev.Volume) * 100
	}

	// 均线极度粘合过滤（在穿心根 len-2 上）：相邻均线间距全部低于阈值时视为粘合，放弃。
	if glued(lines, glueThreshold(opt.Interval)) {
		return model.Result{}, false
	}

	// 方向匹配的均线排列过滤：以最新根 len-1 为结尾的最近 5 根，至少 4 根呈方向排列。
	bull := dir == Up
	arranged := 0
	for i := arrLast - 4; i <= arrLast; i++ {
		vals := []float64{ema5[i], ema10[i], ema30[i], ema60[i], ema120[i]}
		if hasArrangement(vals, bull) {
			arranged++
		}
	}
	if arranged < 4 {
		return model.Result{}, false
	}

	// 强势标记：穿满 5 根，或穿 4 根且 EMA120 在强势一侧。
	strong := false
	switch {
	case crossed == 5:
		strong = true
	case crossed == 4 && dir == Up && ema120[last] <= lo:
		strong = true // 上穿 4 根，EMA120 在实体下方
	case crossed == 4 && dir == Down && ema120[last] >= hi:
		strong = true // 下穿 4 根，EMA120 在实体上方
	}

	dirLabel := "上穿"
	if dir == Down {
		dirLabel = "下穿"
	}
	snap := model.Snapshot{
		Date:       k.Date.Format("2006-01-02 15:04"),
		Close:      k.Close,
		EMA5:       ema5[last],
		EMA10:      ema10[last],
		EMA30:      ema30[last],
		EMA60:      ema60[last],
		EMA120:     ema120[last],
		Bars:       n,
		Volume:     k.Volume,
		PrevVolume: prev.Volume,
	}
	if dir == Up {
		snap.VolumeIncrease = volInc
	}

	special := ""
	if strong {
		special = "·★强势"
	}
	tag := fmt.Sprintf("[%s·%s·穿%d线%s]", intervalLabel(opt.Interval), dirLabel, crossed, special)

	metric := fmt.Sprintf("穿 %d 线", crossed)
	if dir == Up {
		metric = fmt.Sprintf("穿 %d 线, 放量 %.1f%%", crossed, volInc)
	}

	return model.Result{
		Code:     stock.Code,
		Name:     stock.Name,
		Tag:      tag,
		Metric:   metric,
		Alert:    strong,
		Snapshot: snap,
	}, true
}

// glueThreshold 返回相邻均线粘合的百分比阈值：1h 0.1%、4h 0.4%，其余不做粘合过滤（返回 0）。
func glueThreshold(interval string) float64 {
	switch interval {
	case "1h":
		return 0.1
	case "4h":
		return 0.4
	default:
		return 0
	}
}

// glued 判定五条均线是否极度粘合：相邻间距（EMA5-10/10-30/30-60/60-120）全部低于阈值。
// 阈值 <= 0 时视为不过滤（始终返回 false）。
func glued(lines [5]float64, threshold float64) bool {
	if threshold <= 0 {
		return false
	}
	for i := 0; i+1 < len(lines); i++ {
		if gapPct(lines[i], lines[i+1]) >= threshold {
			return false
		}
	}
	return true
}

// hasArrangement 判定按快→慢排列的均线值中，是否存在任意 4 根呈严格方向排列。
// bull=true 要求存在长度 >= 4 的严格递减子序列（多头：快线在上）；否则为递增（空头）。
func hasArrangement(vals []float64, bull bool) bool {
	n := len(vals)
	best := 0
	dp := make([]int, n)
	for i := 0; i < n; i++ {
		dp[i] = 1
		for j := 0; j < i; j++ {
			ordered := vals[j] > vals[i]
			if !bull {
				ordered = vals[j] < vals[i]
			}
			if ordered && dp[j]+1 > dp[i] {
				dp[i] = dp[j] + 1
			}
		}
		if dp[i] > best {
			best = dp[i]
		}
	}
	return best >= 4
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

func intervalLabel(interval string) string {
	switch interval {
	case "15m":
		return "15分钟"
	case "1h":
		return "1小时"
	case "4h":
		return "4小时"
	default:
		return interval
	}
}
