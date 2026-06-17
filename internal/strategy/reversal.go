package strategy

import (
	"fmt"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

// Reversal 实现"超跌拐点"形态：长期空头排列后，关键均线对在近期发生金叉/死叉交织。
// 该形态与周期无关，可作用于周线（周线超跌拐点）或日线（日线超跌拐点）。
type Reversal struct {
	MinBarsNew  int // 新标的最少根数，不足则淘汰
	OldBars     int // 老标的门槛：达到后改用 EMA60/EMA120 这一对均线
	CrossWindow int // 近 N 根内必须发生过交叉
}

// newReversal 按周期给出合适的默认参数。
// 周线沿用原 60/120 周门槛；日线按交易日近似放大（约半年 / 一年）。
func newReversal(p Period) *Reversal {
	switch p.Name() {
	case "day":
		return &Reversal{MinBarsNew: 120, OldBars: 250, CrossWindow: 5}
	default: // week
		return &Reversal{MinBarsNew: 60, OldBars: 120, CrossWindow: 3}
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

	// 1) 当根严格空头排列
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

	// 2) 最近 N 根内对应均线对发生过金叉/死叉
	from := last - r.CrossWindow + 1
	if from < 1 {
		from = 1
	}
	var crossed bool
	if isOld {
		crossed = indicator.Cross(ema60, ema120, from, last)
	} else {
		crossed = indicator.Cross(ema30, ema60, from, last)
	}
	if !crossed {
		return model.Result{}, false
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
