package strategy

import (
	"github.com/find-assets/scanner/internal/aggregator"
	"github.com/find-assets/scanner/internal/model"
)

// Period 表示一种 K 线周期（即指标在哪种 K 线序列上计算）。
// 它只负责把原始日线重采样成目标周期，不关心命中逻辑。
type Period interface {
	Name() string                               // 周期标识，例如 "day" / "week"
	Label() string                              // 中文名，例如 "日线" / "周线"
	Resample(daily []model.Kline) []model.Kline // 把原始日线转换为目标周期 K 线
}

// Minute15Period 表示外部数据源已直接返回 15 分钟 K 线。
type Minute15Period struct{}

func (Minute15Period) Name() string                              { return "15m" }
func (Minute15Period) Label() string                             { return "15分钟" }
func (Minute15Period) Resample(bars []model.Kline) []model.Kline { return bars }

// DayPeriod 日线周期：直接使用原始日线，不做聚合。
type DayPeriod struct{}

func (DayPeriod) Name() string                               { return "day" }
func (DayPeriod) Label() string                              { return "日线" }
func (DayPeriod) Resample(daily []model.Kline) []model.Kline { return daily }

// WeekPeriod 周线周期：在本地把日线合成为周线。
type WeekPeriod struct{}

func (WeekPeriod) Name() string  { return "week" }
func (WeekPeriod) Label() string { return "周线" }
func (WeekPeriod) Resample(daily []model.Kline) []model.Kline {
	return aggregator.ToWeekly(daily)
}
