package strategy

import (
	"fmt"

	"github.com/find-assets/scanner/internal/model"
)

// Strategy 是一次具体扫描所用的策略，由「周期 × 形态」组合而成。
// 周期决定指标算在哪种 K 线上，形态决定怎样才算命中，二者正交、可自由搭配。
type Strategy interface {
	Period() string  // 周期标识，例如 "day" / "week"
	Pattern() string // 形态标识，例如 "pierce" / "reversal"
	Mode() string    // 组合标识，形如 "day:pierce"，用于展示
	Title() string   // 中文标题，例如 "日线一箭穿心"
	Match(stock model.Stock, daily []model.Kline) (model.Result, bool)
}

// Options 构造策略时的可调参数（仅部分形态会用到）。
type Options struct {
	Range     float64 // pierce 形态：均线粘合度阈值（百分比）
	Volume    float64 // pierce 形态：放量阈值（百分比）
	DeadCross int     // reversal 形态：死叉后第几根 K 线触发（默认 3）
}

// combo 把一个 Period 与一个 Pattern 组合成可执行的 Strategy。
type combo struct {
	period  Period
	pattern Pattern
}

func (c combo) Period() string  { return c.period.Name() }
func (c combo) Pattern() string { return c.pattern.Name() }
func (c combo) Mode() string    { return c.period.Name() + ":" + c.pattern.Name() }
func (c combo) Title() string   { return c.period.Label() + c.pattern.Label() }

func (c combo) Match(stock model.Stock, daily []model.Kline) (model.Result, bool) {
	bars := c.period.Resample(daily)
	return c.pattern.Eval(stock, bars)
}

// Periods 返回所有内置周期，供 CLI / API 列举。
func Periods() []Info {
	return []Info{
		{Name: "15m", Label: "15分钟"},
		{Name: "day", Label: "日线"},
		{Name: "week", Label: "周线"},
	}
}

// Patterns 返回所有内置形态，供 CLI / API 列举。
func Patterns() []Info {
	return []Info{
		{Name: "pierce", Label: "一箭穿心"},
		{Name: "reversal", Label: "超跌拐点"},
	}
}

func resolvePeriod(name string) Period {
	switch name {
	case "15m":
		return Minute15Period{}
	case "day":
		return DayPeriod{}
	case "week":
		return WeekPeriod{}
	default:
		return nil
	}
}

func resolvePattern(name string, p Period, opt Options) Pattern {
	switch name {
	case "pierce":
		pc := newPierce(p)
		if opt.Range > 0 {
			pc.Range = opt.Range
		}
		if opt.Volume > 0 {
			pc.Volume = opt.Volume
		}
		return pc
	case "reversal":
		rv := newReversal(p)
		if opt.DeadCross > 0 {
			rv.DeadCrossOffset = opt.DeadCross
		}
		return rv
	default:
		return nil
	}
}

// Get 按「周期 + 形态」构造策略实例；任一维度未知则返回错误。
func Get(period, pattern string, opt Options) (Strategy, error) {
	pd := resolvePeriod(period)
	if pd == nil {
		return nil, fmt.Errorf("未知周期: %q（可选 day | week）", period)
	}
	pt := resolvePattern(pattern, pd, opt)
	if pt == nil {
		return nil, fmt.Errorf("未知形态: %q（可选 pierce | reversal）", pattern)
	}
	return combo{period: pd, pattern: pt}, nil
}
