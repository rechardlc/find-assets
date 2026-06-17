package strategy

import (
	"fmt"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

// Pierce 实现"一箭穿心"形态：EMA 高度粘合后，一根放量阳线同时穿透均线带上下沿。
// 该形态与周期无关，可作用于日线（日线一箭穿心）或周线（周线一箭穿心）。
type Pierce struct {
	MinBars int     // 最少 K 线根数（保证 EMA120 有意义），默认 125
	Range   float64 // 粘合度阈值（百分比），默认 2
	Volume  float64 // 放量阈值（百分比），默认 20，表示较前一根至少放量 20%
}

// newPierce 按周期给出合适的默认参数。
func newPierce(p Period) *Pierce {
	// 日线与周线对 MinBars 取值相同（均需 125 根以使 EMA120 收敛），
	// 语义分别为"约半年交易日"与"约两年半"，可按需通过 Options 覆盖。
	return &Pierce{MinBars: 125, Range: 2, Volume: 20}
}

func (p *Pierce) Name() string  { return "pierce" }
func (p *Pierce) Label() string { return "一箭穿心" }

func (p *Pierce) Eval(stock model.Stock, bars []model.Kline) (model.Result, bool) {
	if len(bars) < p.MinBars {
		return model.Result{}, false
	}
	closes := model.Closes(bars)
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
	emaRange := (maxE - minE) / minE
	if emaRange > p.Range/100 {
		return model.Result{}, false
	}

	k := bars[last]
	if !(k.Close > k.Open) { // 阳线
		return model.Result{}, false
	}
	if !(k.High > maxE && k.Low < minE) { // 一箭穿心
		return model.Result{}, false
	}
	prev := bars[last-1]
	if prev.Volume <= 0 {
		return model.Result{}, false
	}
	volumeIncrease := (float64(k.Volume) - float64(prev.Volume)) / float64(prev.Volume) * 100
	if volumeIncrease < p.Volume {
		return model.Result{}, false
	}

	return model.Result{
		Code:   stock.Code,
		Name:   stock.Name,
		Tag:    "一箭穿心",
		Metric: fmt.Sprintf("粘合度: %.2f%%, 放量: %.2f%%", emaRange*100, volumeIncrease),
		Snapshot: model.Snapshot{
			Date:           k.Date.Format("2006-01-02"),
			Close:          k.Close,
			EMA5:           ema5[last],
			EMA10:          ema10[last],
			EMA30:          ema30[last],
			EMA60:          ema60[last],
			EMA120:         ema120[last],
			Range:          emaRange * 100,
			Volume:         k.Volume,
			PrevVolume:     prev.Volume,
			VolumeIncrease: volumeIncrease,
		},
	}, true
}
