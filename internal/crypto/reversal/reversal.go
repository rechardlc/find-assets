package reversal

import (
	"fmt"
	"math"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

// Direction 拐点方向：超跌（死叉后空头排列）或超涨（金叉后多头排列）。
type Direction string

const (
	Oversold   Direction = "oversold"
	Overbought Direction = "overbought"
)

// Options 数字货币拐点策略参数（与 A 股 reversal 独立，专用于 crypto）。
type Options struct {
	MinBarsNew  int
	OldBars     int
	CrossOffset int     // 交叉后第几根 K 线触发（2 = 第二根）
	MinGapPct   float64 // 相邻均线最小间距（百分比）
	Interval    string  // 周期标识，用于 Tag，例如 "15m"
}

func DefaultOptions(interval string) Options {
	return Options{
		MinBarsNew:  120,
		OldBars:     250,
		CrossOffset: 2,
		MinGapPct:   1,
		Interval:    interval,
	}
}

// MinRequiredBars 返回参与拐点判定所需的最少 K 线根数。
func MinRequiredBars(opt Options) int {
	if opt.MinBarsNew <= 0 {
		return 120
	}
	return opt.MinBarsNew
}

// Eval 在已收盘 K 线序列上判定是否命中指定方向的拐点。
func Eval(stock model.Stock, bars []model.Kline, dir Direction, opt Options) (model.Result, bool) {
	if opt.MinBarsNew <= 0 {
		opt.MinBarsNew = 120
	}
	if opt.OldBars <= 0 {
		opt.OldBars = 250
	}
	if opt.CrossOffset <= 0 {
		opt.CrossOffset = 2
	}
	if opt.MinGapPct <= 0 {
		opt.MinGapPct = 1
	}

	n := len(bars)
	if n < opt.MinBarsNew {
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

	// 末根为当前周期形成中的 K 线，判定对象取倒数第二根已收盘 K 线（len-2）。
	last := n - 2
	if last < 1 {
		return model.Result{}, false
	}
	crossIdx := last - opt.CrossOffset
	if crossIdx < 1 {
		return model.Result{}, false
	}

	if gapPct(ema10[last], ema30[last]) < opt.MinGapPct ||
		gapPct(ema30[last], ema60[last]) < opt.MinGapPct {
		return model.Result{}, false
	}

	fast, slow := ema60, ema120
	if !isOld {
		fast, slow = ema30, ema60
	}

	switch dir {
	case Oversold:
		if !indicator.DeadCrossAt(fast, slow, crossIdx) {
			return model.Result{}, false
		}
		if isOld {
			if !(ema5[last] < ema10[last] &&
				ema10[last] < ema30[last] &&
				ema30[last] < ema60[last] &&
				ema60[last] < ema120[last]) {
				return model.Result{}, false
			}
		} else if !(ema5[last] < ema10[last] &&
			ema10[last] < ema30[last] &&
			ema30[last] < ema60[last]) {
			return model.Result{}, false
		}
	case Overbought:
		if !indicator.GoldenCrossAt(fast, slow, crossIdx) {
			return model.Result{}, false
		}
		if isOld {
			if !(ema5[last] > ema10[last] &&
				ema10[last] > ema30[last] &&
				ema30[last] > ema60[last] &&
				ema60[last] > ema120[last]) {
				return model.Result{}, false
			}
		} else if !(ema5[last] > ema10[last] &&
			ema10[last] > ema30[last] &&
			ema30[last] > ema60[last]) {
			return model.Result{}, false
		}
	default:
		return model.Result{}, false
	}

	snap := model.Snapshot{
		Date:  bars[last].Date.Format("2006-01-02 15:04"),
		Close: bars[last].Close,
		EMA5:  ema5[last],
		EMA10: ema10[last],
		EMA30: ema30[last],
		EMA60: ema60[last],
		Bars:  n,
	}
	dirLabel := "超跌拐点"
	if dir == Overbought {
		dirLabel = "超涨拐点"
	}
	ageLabel := "新币"
	if isOld {
		snap.EMA120 = ema120[last]
		ageLabel = "老币"
	}
	tag := fmt.Sprintf("[%s·%s·%s]", intervalLabel(opt.Interval), dirLabel, ageLabel)

	return model.Result{
		Code:     stock.Code,
		Name:     stock.Name,
		Tag:      tag,
		Metric:   fmt.Sprintf("样本 %d 根", n),
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

func gapPct(a, b float64) float64 {
	hi := math.Max(a, b)
	if hi == 0 {
		return 0
	}
	return math.Abs(a-b) / hi * 100
}
