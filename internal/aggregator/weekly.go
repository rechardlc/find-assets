package aggregator

import (
	"github.com/find-assets/scanner/internal/model"
)

// ToWeekly 将按日期升序排列的日线合并为周线。
// 周的划分采用 ISO 周编号（周一为一周起点），与 A 股惯例一致。
// 当周即使尚未收盘也会形成一根（最后一根可能是"伪 K 线"），
// 这是策略所要求的"含当周"语义。
func ToWeekly(daily []model.Kline) []model.Kline {
	if len(daily) == 0 {
		return nil
	}
	out := make([]model.Kline, 0, len(daily)/5+1)

	var cur model.Kline
	var curY, curW int
	started := false

	for _, d := range daily {
		y, w := d.Date.ISOWeek()
		if !started || y != curY || w != curW {
			if started {
				out = append(out, cur)
			}
			cur = model.Kline{
				Date:   d.Date,
				Open:   d.Open,
				High:   d.High,
				Low:    d.Low,
				Close:  d.Close,
				Volume: d.Volume,
			}
			curY, curW = y, w
			started = true
			continue
		}
		if d.High > cur.High {
			cur.High = d.High
		}
		if d.Low < cur.Low {
			cur.Low = d.Low
		}
		cur.Close = d.Close
		cur.Volume += d.Volume
		cur.Date = d.Date
	}
	if started {
		out = append(out, cur)
	}
	return out
}
