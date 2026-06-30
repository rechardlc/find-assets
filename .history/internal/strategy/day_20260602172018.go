package strategy

import (
	"fmt"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

// Day 实现"日线一箭穿心"策略。
type Day struct {
	MinBars  int     // 上市天数要求（按交易日近似），默认 125
	Range float64 // 粘合度阈值（百分比），默认 1.5
}

func NewDay() *Day {
	return &Day{MinBars: 125, Range: 1.5}
}

func (d *Day) Mode() string  { return "day" }
func (d *Day) Title() string { return "日线一箭穿心" }

func (d *Day) Match(stock model.Stock, daily []model.Kline) (model.Result, bool) {
	if len(daily) < d.MinBars {
		return model.Result{}, false
	}
	closes := model.Closes(daily)
	ema5 := indicator.EMA(closes, 5)
	ema10 := indicator.EMA(closes, 10)
	ema30 := indicator.EMA(closes, 30)
	ema60 := indicator.EMA(closes, 60)
	ema120 := indicator.EMA(closes, 120)

	last := len(closes) - 1
	vals := [5]float64{ema5[last], ema10[last], ema30[last], ema60[last], ema120[last]}

	maxE, minE := vals[0], vals[0]
	for _, v := range vals[1:] {
		if v > maxE {
			maxE = v
		}
		if v < minE {
			minE = v
		}
	}
	if minE <= 0 {
		return model.Result{}, false
	}
	ratio := (maxE - minE) / minE
	if ratio > d.Range/100 {
		return model.Result{}, false
	}

	k := daily[last]
	if !(k.Close > k.Open) { // 阳线
		return model.Result{}, false
	}
	if !(k.High > maxE && k.Low < minE) { // 一箭穿心
		return model.Result{}, false
	}

	return model.Result{
		Code:   stock.Code,
		Name:   stock.Name,
		Tag:    "[日线穿心突破]",
		Metric: fmt.Sprintf("粘合度: %.2f%%", ratio*100),
		Snapshot: model.Snapshot{
			Date:     k.Date.Format("2006-01-02"),
			Close:    k.Close,
			EMA5:     ema5[last],
			EMA10:    ema10[last],
			EMA30:    ema30[last],
			EMA60:    ema60[last],
			EMA120:   ema120[last],
			Range:    ratio * 100,
		},
	}, true
}
