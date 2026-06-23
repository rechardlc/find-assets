package strategy

import (
	"fmt"
	"math"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

// Reversal 实现"超跌拐点"形态：长期均线发生死叉后，价格在第三根 K 线上仍维持
// 空头排列、且相邻均线之间保持足够间距（避免均线粘合的假信号）。
// 该形态与周期无关，可作用于周线（周线超跌拐点）或日线（日线超跌拐点）。
type Reversal struct {
	MinBarsNew      int     // 新标的最少根数，不足则淘汰
	OldBars         int     // 老标的门槛：达到后改用 EMA60/EMA120 这一对均线
	DeadCrossOffset int     // 死叉后第几根 K 线触发（3 = 死叉后第三根，即当根）
	MinGapPct       float64 // 相邻均线最小间距（百分比，1 = 1%）
}

// newReversal 按周期给出合适的默认参数。
// 周线沿用原 60/120 周门槛；日线按交易日近似放大（约半年 / 一年）。
func newReversal(p Period) *Reversal {
	switch p.Name() {
	case "day":
		return &Reversal{MinBarsNew: 120, OldBars: 250, DeadCrossOffset: 3, MinGapPct: 1}
	default: // week
		return &Reversal{MinBarsNew: 60, OldBars: 120, DeadCrossOffset: 3, MinGapPct: 1}
	}
}

func (r *Reversal) Name() string  { return "reversal" }
func (r *Reversal) Label() string { return "超跌拐点" }

func (r *Reversal) Eval(stock model.Stock, bars []model.Kline) (model.Result, bool) {
	n := len(bars)
	if n < r.MinBarsNew {
		return model.Result{}, false
	}

	closes := model.Closes(bars)
	ema5 := indicator.EMA(closes, 5)
	ema10 := indicator.EMA(closes, 10)
	ema30 := indicator.EMA(closes, 30)
	ema60 := indicator.EMA(closes, 60)

	isOld := n >= r.OldBars
	var ema120 []float64
	if isOld {
		ema120 = indicator.EMA(closes, 120)
	}

	last := n - 1

	// 死叉所在根：当根（last）须正好是死叉后第 DeadCrossOffset 根。
	crossIdx := last - r.DeadCrossOffset
	if crossIdx < 1 {
		return model.Result{}, false
	}

	// 1) 相邻均线间距：EMA10/EMA30、EMA30/EMA60 均需相差 MinGapPct 以上，
	//    避免均线粘合时被误判为超跌拐点。
	if gapPct(ema10[last], ema30[last]) < r.MinGapPct ||
		gapPct(ema30[last], ema60[last]) < r.MinGapPct {
		return model.Result{}, false
	}

	// 2) 死叉后第三根：老股看 EMA60/EMA120，新股用 EMA30/EMA60 平替；
	//    且当根呈严格空头排列。
	fast, slow := ema60, ema120
	if !isOld {
		fast, slow = ema30, ema60
	}
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
	} else {
		if !(ema5[last] < ema10[last] &&
			ema10[last] < ema30[last] &&
			ema30[last] < ema60[last]) {
			return model.Result{}, false
		}
	}

	snap := model.Snapshot{
		Date:  bars[last].Date.Format("2006-01-02"),
		Close: bars[last].Close,
		EMA5:  ema5[last],
		EMA10: ema10[last],
		EMA30: ema30[last],
		EMA60: ema60[last],
		Bars:  n,
	}
	tag := "[超跌拐点·新股]"
	if isOld {
		snap.EMA120 = ema120[last]
		tag = "[超跌拐点·老股]"
	}

	return model.Result{
		Code:     stock.Code,
		Name:     stock.Name,
		Tag:      tag,
		Metric:   fmt.Sprintf("样本 %d 根", n),
		Snapshot: snap,
	}, true
}

// gapPct 返回两条均线的相对间距百分比，以较大值为基准。
func gapPct(a, b float64) float64 {
	hi := math.Max(a, b)
	if hi == 0 {
		return 0
	}
	return math.Abs(a-b) / hi * 100
}
