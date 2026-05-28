package strategy

import "github.com/find-assets/scanner/internal/model"

// Strategy 定义一种选股策略。
type Strategy interface {
	Mode() string  // 模式名，例如 "day" / "week"
	Title() string // 中文标题，用于报告
	Match(stock model.Stock, daily []model.Kline) (model.Result, bool)
}

// Get 根据模式名获取内置策略实例；返回 nil 表示未注册。
func Get(mode string) Strategy {
	switch mode {
	case "day":
		return NewDay()
	case "week":
		return NewWeek()
	default:
		return nil
	}
}
