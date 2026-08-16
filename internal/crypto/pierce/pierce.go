// Package pierce 实现数字货币专用的「一箭穿心」形态判定，与 A 股 internal/strategy 完全独立。
//
// 判定规则（仅作用于有 EMA120 的老币，新币直接舍弃）：
//   - 判定对象为**倒数第二根已收盘 K 线**（len-2）；序列末尾（len-1）为当前周期在形成中的 K 线
//   - 双向：上穿（阳线自下而上跨过均线）/ 下穿（阴线自上而下跨过均线）
//   - 「实体跨越」：被穿的均线严格夹在开盘价与收盘价之间
//   - 粘合为必要条件：EMA5/10/30/60/120 中存在一组 4 根，组内任意两两间距 < 周期阈值，
//     且这 4 根都被实体穿过（15m=0.04%，1h=0.08%，4h=1%）
//   - 上穿要求放量（当根成交量 > 前一根），下穿不要求
//   - 不做方向排列过滤
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

// 各周期 5 选 4 均线带宽上限（百分比）：组内 (max-min)/max * 100 须严格小于该值。
const (
	GluePct15m = 0.04
	GluePct1h  = 0.08
	GluePct4h  = 1.0
)

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

// Eval 判定一箭穿心：穿心与粘合均在倒数第二根已收盘 K 线（len-2）。
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

	last := n - 2
	if last-1 < 0 {
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

	// 粘合必要条件：存在一组 4 根均线两两间距低于阈值，且这 4 根都被实体穿过。
	if !hasGluedPiercedFour(lines, lo, hi, glueThreshold(opt.Interval)) {
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

// glueThreshold 返回 5 选 4 均线带宽上限（百分比）。未知周期返回 0（粘合条件无法满足）。
func glueThreshold(interval string) float64 {
	switch interval {
	case "15m":
		return GluePct15m
	case "1h":
		return GluePct1h
	case "4h":
		return GluePct4h
	default:
		return 0
	}
}

// hasGluedPiercedFour 判定是否存在一组 4 根均线：组内 (max-min)/max*100 < threshold，且均被 (lo, hi) 严格穿过。
func hasGluedPiercedFour(lines [5]float64, lo, hi, threshold float64) bool {
	if threshold <= 0 {
		return false
	}
	for skip := 0; skip < len(lines); skip++ {
		minV, maxV := 0.0, 0.0
		first := true
		ok := true
		for i, v := range lines {
			if i == skip {
				continue
			}
			if !(lo < v && v < hi) {
				ok = false
				break
			}
			if first {
				minV, maxV, first = v, v, false
				continue
			}
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
		if !ok || maxV <= 0 {
			continue
		}
		if (maxV-minV)/maxV*100 < threshold {
			return true
		}
	}
	return false
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
