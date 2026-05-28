package strategy

import (
	"fmt"

	"github.com/find-assets/scanner/internal/aggregator"
	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

// Week 实现"周线超跌拐点"策略。
type Week struct {
	MinWeeksNew int // 新股最少周数，默认 60
	OldWeeks    int // 老股门槛，默认 120
	CrossWindow int // 近 N 周内必须发生过交叉，默认 3
}

func NewWeek() *Week {
	return &Week{MinWeeksNew: 60, OldWeeks: 120, CrossWindow: 3}
}

func (w *Week) Mode() string  { return "week" }
func (w *Week) Title() string { return "周线超跌拐点" }

func (w *Week) Match(stock model.Stock, daily []model.Kline) (model.Result, bool) {
	weekly := aggregator.ToWeekly(daily)
	n := len(weekly)
	if n < w.MinWeeksNew {
		return model.Result{}, false
	}

	closes := model.Closes(weekly)
	ema5 := indicator.EMA(closes, 5)
	ema10 := indicator.EMA(closes, 10)
	ema30 := indicator.EMA(closes, 30)
	ema60 := indicator.EMA(closes, 60)

	isOld := n >= w.OldWeeks
	var ema120 []float64
	if isOld {
		ema120 = indicator.EMA(closes, 120)
	}

	last := n - 1

	// 1) 当周严格空头排列
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

	// 2) 最近 N 周内对应均线对发生过金叉/死叉
	from := last - w.CrossWindow + 1
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
		Date:  weekly[last].Date.Format("2006-01-02"),
		Close: weekly[last].Close,
		EMA5:  ema5[last],
		EMA10: ema10[last],
		EMA30: ema30[last],
		EMA60: ema60[last],
		Weeks: n,
	}
	tag := "[周线超跌拐点·新股]"
	if isOld {
		snap.EMA120 = ema120[last]
		tag = "[周线超跌拐点·老股]"
	}

	return model.Result{
		Code:     stock.Code,
		Name:     stock.Name,
		Tag:      tag,
		Metric:   fmt.Sprintf("上市 %d 周", n),
		Snapshot: snap,
	}, true
}
