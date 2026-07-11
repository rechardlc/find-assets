// Package pierce 实现数字货币专用的「一箭穿心」形态判定，与 A 股 internal/strategy 完全独立。
//
// 与 A 股一箭穿心的差异：
//   - 双向：上穿（阳线自下而上跨过均线）/ 下穿（阴线自上而下跨过均线）
//   - 判定「实体跨越」：被穿的均线夹在开盘价与收盘价之间（实体整体跨过这些线）
//   - 需穿 EMA5/10/30/60/120 中任意 4 根；无 EMA120 的新币须穿满 EMA5/10/30/60，不足 4 根放弃
//   - 上穿要求放量（当根成交量 > 前一根），下穿不要求
//   - 不做均线粘合度过滤
//   - 特殊标记：上穿且穿满 EMA5/10/30/60、EMA120 位于最低价下方 → 强多头信号（Alert）
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
	MinBarsNew int    // 最少 K 线（新币，仅 EMA5/10/30/60），默认 120
	OldBars    int    // 达到该根数才启用 EMA120（老币），默认 250
	MinCross   int    // 至少跨越几根均线，默认 4
	Interval   string // 周期标识，用于 Tag，例如 "1h"
}

func DefaultOptions(interval string) Options {
	return Options{MinBarsNew: 120, OldBars: 250, MinCross: 4, Interval: interval}
}

// MinRequiredBars 返回参与判定所需的最少 K 线根数。
func MinRequiredBars(opt Options) int {
	if opt.MinBarsNew <= 0 {
		return 120
	}
	return opt.MinBarsNew
}

// Eval 在已收盘 K 线序列上判定最新一根是否命中指定方向的一箭穿心。
func Eval(stock model.Stock, bars []model.Kline, dir Direction, opt Options) (model.Result, bool) {
	if opt.MinBarsNew <= 0 {
		opt.MinBarsNew = 120
	}
	if opt.OldBars <= 0 {
		opt.OldBars = 250
	}
	if opt.MinCross <= 0 {
		opt.MinCross = 4
	}

	n := len(bars)
	if n < opt.MinBarsNew || n < 2 {
		return model.Result{}, false
	}

	closes := model.Closes(bars)
	ema5 := indicator.EMA(closes, 5)
	ema10 := indicator.EMA(closes, 10)
	ema30 := indicator.EMA(closes, 30)
	ema60 := indicator.EMA(closes, 60)

	isOld := n >= opt.OldBars
	var ema120 []float64
	if isOld {
		ema120 = indicator.EMA(closes, 120)
	}

	last := n - 1
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

	// 统计被实体跨越的均线（严格位于 lo 与 hi 之间）。
	fastVals := [4]float64{ema5[last], ema10[last], ema30[last], ema60[last]}
	crossedFast := 0
	for _, v := range fastVals {
		if lo < v && v < hi {
			crossedFast++
		}
	}
	crossed := crossedFast
	if isOld {
		if v := ema120[last]; lo < v && v < hi {
			crossed++
		}
	}
	// 老币可在 5 根中任选 4 根；新币仅 4 根，须全穿（crossed >= 4 即全穿）。
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

	// 特殊标记：上穿 + 老币 + 穿满 EMA5/10/30/60 + EMA120 位于最低价下方。
	alert := dir == Up && isOld && crossedFast == 4 && ema120[last] < k.Low

	dirLabel := "上穿"
	if dir == Down {
		dirLabel = "下穿"
	}
	ageLabel := "新币"
	snap := model.Snapshot{
		Date:       k.Date.Format("2006-01-02 15:04"),
		Close:      k.Close,
		EMA5:       ema5[last],
		EMA10:      ema10[last],
		EMA30:      ema30[last],
		EMA60:      ema60[last],
		Bars:       n,
		Volume:     k.Volume,
		PrevVolume: prev.Volume,
	}
	if isOld {
		ageLabel = "老币"
		snap.EMA120 = ema120[last]
	}
	if dir == Up {
		snap.VolumeIncrease = volInc
	}

	special := ""
	if alert {
		special = "·★特殊多头"
	}
	tag := fmt.Sprintf("[%s·%s·%s·穿%d线%s]", intervalLabel(opt.Interval), dirLabel, ageLabel, crossed, special)

	metric := fmt.Sprintf("穿 %d 线", crossed)
	if dir == Up {
		metric = fmt.Sprintf("穿 %d 线, 放量 %.1f%%", crossed, volInc)
	}

	return model.Result{
		Code:     stock.Code,
		Name:     stock.Name,
		Tag:      tag,
		Metric:   metric,
		Alert:    alert,
		Snapshot: snap,
	}, true
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
