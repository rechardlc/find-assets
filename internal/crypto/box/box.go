// Package box 实现数字货币专用的「箱体震荡」形态判定，用于捕捉被反复验证的支撑 / 阻力。
//
// 判定规则：
//   - 判定基准根为**上一根已收盘 K 线**（len-2）；序列末尾（len-1）为当前周期仍在形成中的 K 线，不参与判定
//   - 回看窗口为以 len-2 结尾、最多 Lookback 根（默认 24）已收盘 K 线
//   - 底部箱体：窗口内某个区间的最低价 L 为下沿，最低价落在 [L, L*(1+0.6%)] 的 K 线记为一次「触及」，
//     触及 >= 3 次即认定支撑被反复验证；因 L 取自区间最低价，「区间内不得跌破下沿」天然成立
//   - 顶部箱体：与底部对称，改用最高价，上沿 H 取区间最高价，成员区间为 [H*(1-0.6%), H]
//   - **末根已收盘 K 线必须是箱体成员**：箱体走完、价格已脱离时信号视为过期，不再命中
//   - **首末两次触及之间至少隔 MinGap 根中间 K 线**（默认 6，即含端点跨度 >= 8）：
//     连续几根挤在一起不算箱体震荡，支撑 / 阻力要跨越足够时间被重新验证才有意义
//   - **箱体内振幅至少 MinAmpPct**（默认 5%）：箱体内（首次触及到末根之间的全部 K 线）
//     最高 High 与最低 Low 相差不足该值时，只是贴着均价的死水横盘，
//     支撑 / 阻力没被真正来回考验，也没有可交易空间
//   - 多个起点都成立时取触及次数最多者；次数相同取起点最靠后者（最紧凑、最新鲜的箱体）
//   - 强势标记（Alert）：触及次数 >= AlertTouches（默认 5），同一价位被反复验证越多，突破时越值得关注
package box

import (
	"fmt"
	"strconv"

	"github.com/find-assets/scanner/internal/model"
)

// 默认参数。
const (
	DefaultPct          = 0.6 // 箱体带宽上限（百分比）
	DefaultLookback     = 24  // 回看已收盘 K 线根数
	DefaultTouches      = 3   // 最少触及次数
	DefaultMinGap       = 6   // 首末触及之间的中间 K 线最少根数
	DefaultAlertTouches = 5   // 强势标记所需触及次数
	DefaultMinAmpPct    = 5   // 箱体跨度内振幅下限（百分比）
)

// Direction 箱体方向。
type Direction string

const (
	Bottom Direction = "bottom" // 底部箱体：支撑被反复踩住
	Top    Direction = "top"    // 顶部箱体：阻力被反复压制
)

// Options 箱体震荡参数。
type Options struct {
	MaxWidthPct  float64 // 箱体带宽上限（百分比），默认 0.6
	Lookback     int     // 回看已收盘 K 线根数，默认 24
	MinTouches   int     // 最少触及次数，默认 3
	MinGap       int     // 首末触及之间的中间 K 线最少根数，默认 6
	AlertTouches int     // 触及次数达到该值时标记 ★，默认 5
	MinAmpPct    float64 // 箱体跨度内振幅下限（百分比），默认 5
	Interval     string  // 周期标识，用于 Tag，例如 "1h"
}

func DefaultOptions(interval string) Options {
	return Options{
		MaxWidthPct:  DefaultPct,
		Lookback:     DefaultLookback,
		MinTouches:   DefaultTouches,
		MinGap:       DefaultMinGap,
		AlertTouches: DefaultAlertTouches,
		MinAmpPct:    DefaultMinAmpPct,
		Interval:     interval,
	}
}

// MinRequiredBars 返回参与判定所需的最少 K 线根数：能容纳一个最小箱体的已收盘根数 + 1 根形成中的末根。
// 回看根数只是窗口上限，历史不足的新上线合约按实际根数判定。
func MinRequiredBars(opt Options) int {
	return minWindow(normalize(opt)) + 1
}

// minWindow 返回箱体所需的最少已收盘 K 线根数：既要放得下 MinTouches 次触及，
// 也要放得下「首触及 + MinGap 根中间 K 线 + 末触及」的跨度。
func minWindow(opt Options) int {
	if span := opt.MinGap + 2; span > opt.MinTouches {
		return span
	}
	return opt.MinTouches
}

// zone 一个成立的箱体：[lo, hi] 为成员价格实际覆盖的区间，first 为首次触及的下标，
// amp 为跨度 [first, last] 内的振幅（百分比）。
type zone struct {
	first   int
	touches int
	lo      float64
	hi      float64
	amp     float64
}

// Eval 判定末根已收盘 K 线是否正处于一个被反复验证的箱体上沿 / 下沿。
func Eval(stock model.Stock, bars []model.Kline, dir Direction, opt Options) (model.Result, bool) {
	if dir != Bottom && dir != Top {
		return model.Result{}, false
	}
	opt = normalize(opt)

	n := len(bars)
	if n < MinRequiredBars(opt) {
		return model.Result{}, false
	}
	last := n - 2
	from := last - opt.Lookback + 1
	if from < 0 {
		from = 0
	}
	if last-from+1 < minWindow(opt) {
		return model.Result{}, false
	}
	for i := from; i <= last; i++ {
		if bars[i].Low <= 0 || bars[i].High < bars[i].Low {
			return model.Result{}, false // 数据异常，放弃该合约
		}
	}

	best, ok := findZone(bars, from, last, dir, opt)
	if !ok {
		return model.Result{}, false
	}

	k := bars[last]
	widthPct := (best.hi - best.lo) / best.lo * 100
	strong := best.touches >= opt.AlertTouches

	dirLabel, distLabel := "底部箱体", "距下沿"
	distPct := (k.Close - best.lo) / best.lo * 100
	if dir == Top {
		dirLabel, distLabel = "顶部箱体", "距上沿"
		distPct = (best.hi - k.Close) / best.hi * 100
	}

	special := ""
	if strong {
		special = "·★强势"
	}
	tag := fmt.Sprintf("[%s·%s·触及%d次·带宽%.2f%%%s]", intervalLabel(opt.Interval), dirLabel, best.touches, widthPct, special)

	metric := fmt.Sprintf("箱体 %s~%s, 触及 %d 次, 跨 %d 根, 振幅 %.2f%%, %s %.2f%%",
		formatPrice(best.lo), formatPrice(best.hi), best.touches, last-best.first+1, best.amp, distLabel, distPct)

	return model.Result{
		Code:   stock.Code,
		Name:   stock.Name,
		Tag:    tag,
		Metric: metric,
		Alert:  strong,
		Snapshot: model.Snapshot{
			Date:      k.Date.Format("2006-01-02 15:04"),
			Close:     k.Close,
			Low:       best.lo,
			High:      best.hi,
			Range:     widthPct,
			Amplitude: best.amp,
			Touches:   best.touches,
			Bars:      n,
		},
	}, true
}

// MergeSideways 将同币种同时命中的底部 + 顶部箱体合成一条「窄幅横盘」。
// Snapshot.Touches 取两侧较大值，便于报告按触及次数排序；Alert 任一侧重势则保留。
func MergeSideways(bottom, top model.Result, interval string) model.Result {
	lo, hi := bottom.Snapshot.Low, top.Snapshot.High
	heightPct := 0.0
	if lo > 0 {
		heightPct = (hi - lo) / lo * 100
	}
	touches := bottom.Snapshot.Touches
	if top.Snapshot.Touches > touches {
		touches = top.Snapshot.Touches
	}
	amp := bottom.Snapshot.Amplitude
	if top.Snapshot.Amplitude > amp {
		amp = top.Snapshot.Amplitude
	}
	alert := bottom.Alert || top.Alert
	special := ""
	if alert {
		special = "·★强势"
	}
	tag := fmt.Sprintf("[%s·窄幅横盘·底%d次·顶%d次·高宽%.2f%%%s]",
		intervalLabel(interval), bottom.Snapshot.Touches, top.Snapshot.Touches, heightPct, special)
	metric := fmt.Sprintf("横盘 %s~%s, 底触及 %d 次 / 顶触及 %d 次, 高宽 %.2f%%, 振幅 %.2f%%",
		formatPrice(lo), formatPrice(hi), bottom.Snapshot.Touches, top.Snapshot.Touches, heightPct, amp)
	return model.Result{
		Code:   bottom.Code,
		Name:   bottom.Name,
		Tag:    tag,
		Metric: metric,
		Alert:  alert,
		Snapshot: model.Snapshot{
			Date:      bottom.Snapshot.Date,
			Close:     bottom.Snapshot.Close,
			Low:       lo,
			High:      hi,
			Range:     heightPct,
			Amplitude: amp,
			Touches:   touches,
			Bars:      bottom.Snapshot.Bars,
		},
	}
}

// findZone 从末根往前扩展箱体起点，返回触及次数最多的箱体。
// 起点越往前，边沿只会越极端（底部更低 / 顶部更高），因此一旦末根被挤出带宽即可提前收敛。
func findZone(bars []model.Kline, from, last int, dir Direction, opt Options) (zone, bool) {
	tol := opt.MaxWidthPct / 100
	minLow, maxHigh := suffixExtremes(bars, from, last)
	edge := edgePrice(bars[last], dir)
	best := zone{}
	found := false

	for i := last; i >= from; i-- {
		p := edgePrice(bars[i], dir)
		if dir == Bottom && p < edge {
			edge = p
		}
		if dir == Top && p > edge {
			edge = p
		}
		limit := memberLimit(edge, tol, dir)
		if !isMember(edgePrice(bars[last], dir), limit, dir) {
			break
		}

		z := zone{first: -1, lo: edge, hi: edge}
		for j := i; j <= last; j++ {
			q := edgePrice(bars[j], dir)
			if !isMember(q, limit, dir) {
				continue
			}
			if z.first < 0 {
				z.first = j
			}
			z.touches++
			if q < z.lo {
				z.lo = q
			}
			if q > z.hi {
				z.hi = q
			}
		}
		if z.touches < opt.MinTouches {
			continue
		}
		// 首末触及之间的中间 K 线数量门槛：滤掉挤在一起的伪箱体。
		// 起点越往前，首次触及只会越早，间隔单调不减，因此这里 continue 而非 break。
		if last-z.first-1 < opt.MinGap {
			continue
		}
		// 跨度内振幅门槛：滤掉贴着均价的死水横盘。跨度随起点前移只会变宽、振幅单调不减，同样 continue。
		lo, hi := minLow[z.first-from], maxHigh[z.first-from]
		z.amp = (hi - lo) / lo * 100
		if z.amp < opt.MinAmpPct {
			continue
		}
		if z.touches > best.touches {
			best, found = z, true
		}
	}
	return best, found
}

// suffixExtremes 预计算 [i, last] 区间的最低价与最高价，下标以 from 为原点偏移，
// 供各候选箱体按其首次触及下标 O(1) 取得跨度内振幅。
func suffixExtremes(bars []model.Kline, from, last int) (minLow, maxHigh []float64) {
	n := last - from + 1
	minLow = make([]float64, n)
	maxHigh = make([]float64, n)
	for i := last; i >= from; i-- {
		lo, hi := bars[i].Low, bars[i].High
		if i < last {
			if next := minLow[i-from+1]; next < lo {
				lo = next
			}
			if next := maxHigh[i-from+1]; next > hi {
				hi = next
			}
		}
		minLow[i-from], maxHigh[i-from] = lo, hi
	}
	return minLow, maxHigh
}

// edgePrice 返回参与箱体判定的价格：底部箱体看最低价，顶部箱体看最高价。
func edgePrice(k model.Kline, dir Direction) float64 {
	if dir == Top {
		return k.High
	}
	return k.Low
}

// memberLimit 返回成员价格的边界：底部为下沿上浮 tol，顶部为上沿下浮 tol。
func memberLimit(edge, tol float64, dir Direction) float64 {
	if dir == Top {
		return edge * (1 - tol)
	}
	return edge * (1 + tol)
}

func isMember(price, limit float64, dir Direction) bool {
	if dir == Top {
		return price >= limit
	}
	return price <= limit
}

func normalize(opt Options) Options {
	if opt.MaxWidthPct <= 0 {
		opt.MaxWidthPct = DefaultPct
	}
	if opt.Lookback < DefaultTouches {
		opt.Lookback = DefaultLookback
	}
	if opt.MinTouches < DefaultTouches {
		opt.MinTouches = DefaultTouches
	}
	if opt.MinGap <= 0 {
		opt.MinGap = DefaultMinGap
	}
	if opt.AlertTouches <= 0 {
		opt.AlertTouches = DefaultAlertTouches
	}
	if opt.MinAmpPct <= 0 {
		opt.MinAmpPct = DefaultMinAmpPct
	}
	if win := opt.MinGap + 2; opt.Lookback < win {
		opt.Lookback = win
	}
	if opt.Lookback < opt.MinTouches {
		opt.Lookback = opt.MinTouches
	}
	return opt
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

// formatPrice 以最短精确表示输出价格，避免山寨币小数价被定长小数截断。
func formatPrice(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
