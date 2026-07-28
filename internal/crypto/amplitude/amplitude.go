// Package amplitude 实现数字货币专用的「振幅异动」形态判定，用于捕捉情绪涨 / 情绪跌。
//
// 判定规则：
//   - 判定对象为**上一根已收盘 K 线**（len-2）；序列末尾（len-1）为当前周期仍在形成中的 K 线，不参与判定
//   - 振幅 = (最高价 - 最低价) / 最低价 * 100，达到阈值（默认 9%）即命中
//   - 方向由实体决定：收盘 > 开盘 为情绪涨，收盘 < 开盘 为情绪跌，收盘 = 开盘 为情绪分歧
//   - 强势标记（Alert）：振幅 >= 阈值 * AlertMul（默认 2 倍，即 18%）
//   - 不做均线与量能过滤：振幅本身即情绪强度，只需上一根 K 线自身的 OHLC
package amplitude

import (
	"fmt"
	"math"
	"strconv"

	"github.com/find-assets/scanner/internal/model"
)

// 默认参数。
const (
	DefaultMinPct   = 9 // 振幅阈值（百分比）
	DefaultAlertMul = 2 // 强势倍数
	minBarsFloor    = 2 // 至少 2 根才能取到上一根已收盘 K 线
)

// Direction 情绪方向。
type Direction string

const (
	Up   Direction = "up"   // 情绪涨：收盘 > 开盘
	Down Direction = "down" // 情绪跌：收盘 < 开盘
	Flat Direction = "flat" // 情绪分歧：收盘 = 开盘，长上下影线来回扫针
)

// Options 振幅异动参数。
type Options struct {
	MinPct   float64 // 振幅阈值（百分比），默认 9
	AlertMul float64 // 强势倍数：振幅 >= MinPct*AlertMul 时标记 ★，默认 2
	MinBars  int     // 参与判定所需最少 K 线根数，默认 2（不过滤新上线合约）
	Interval string  // 周期标识，用于 Tag，例如 "4h"
}

func DefaultOptions(interval string) Options {
	return Options{
		MinPct:   DefaultMinPct,
		AlertMul: DefaultAlertMul,
		MinBars:  minBarsFloor,
		Interval: interval,
	}
}

// MinRequiredBars 返回参与振幅判定所需的最少 K 线根数。
func MinRequiredBars(opt Options) int {
	if opt.MinBars < minBarsFloor {
		return minBarsFloor
	}
	return opt.MinBars
}

// Eval 判定上一根已收盘 K 线（len-2）的振幅是否达到阈值。
func Eval(stock model.Stock, bars []model.Kline, opt Options) (model.Result, bool) {
	if opt.MinPct <= 0 {
		opt.MinPct = DefaultMinPct
	}
	if opt.AlertMul <= 0 {
		opt.AlertMul = DefaultAlertMul
	}
	if opt.MinBars < minBarsFloor {
		opt.MinBars = minBarsFloor
	}

	n := len(bars)
	if n < opt.MinBars {
		return model.Result{}, false
	}

	// 末根为当前周期形成中的 K 线，判定对象取上一根已收盘 K 线（len-2）。
	last := n - 2
	if last < 0 {
		return model.Result{}, false
	}
	k := bars[last]

	amp, ok := Pct(k)
	if !ok || amp < opt.MinPct {
		return model.Result{}, false
	}

	dir := DirectionOf(k)
	strong := amp >= opt.MinPct*opt.AlertMul

	special := ""
	if strong {
		special = "·★强势"
	}
	tag := fmt.Sprintf("[%s·%s·振幅%.1f%%%s]", intervalLabel(opt.Interval), directionLabel(dir), amp, special)

	metric := fmt.Sprintf("振幅 %.2f%%, 最高 %s, 最低 %s, 实体占比 %.1f%%",
		amp, formatPrice(k.High), formatPrice(k.Low), bodyRatio(k))

	return model.Result{
		Code:   stock.Code,
		Name:   stock.Name,
		Tag:    tag,
		Metric: metric,
		Alert:  strong,
		Snapshot: model.Snapshot{
			Date:      k.Date.Format("2006-01-02 15:04"),
			Close:     k.Close,
			High:      k.High,
			Low:       k.Low,
			Amplitude: amp,
			Volume:    k.Volume,
			Bars:      n,
		},
	}, true
}

// Pct 返回单根 K 线的振幅百分比：(最高价 - 最低价) / 最低价 * 100。
// 最低价非正或高低价倒挂（数据异常）时返回 false。
func Pct(k model.Kline) (float64, bool) {
	if k.Low <= 0 || k.High < k.Low {
		return 0, false
	}
	return (k.High - k.Low) / k.Low * 100, true
}

// DirectionOf 按实体方向判定情绪。
func DirectionOf(k model.Kline) Direction {
	switch {
	case k.Close > k.Open:
		return Up
	case k.Close < k.Open:
		return Down
	default:
		return Flat
	}
}

// bodyRatio 返回实体占振幅的比例（百分比），用于区分单边推动与上下扫针。
func bodyRatio(k model.Kline) float64 {
	span := k.High - k.Low
	if span <= 0 {
		return 0
	}
	return math.Abs(k.Close-k.Open) / span * 100
}

func directionLabel(dir Direction) string {
	switch dir {
	case Up:
		return "情绪涨"
	case Down:
		return "情绪跌"
	default:
		return "情绪分歧"
	}
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

// formatPrice 以最短精确表示输出价格，避免山寨币小数价被科学计数法或定长小数截断。
func formatPrice(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
