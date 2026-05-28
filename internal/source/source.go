package source

import (
	"context"

	"github.com/find-assets/scanner/internal/model"
)

// Source 抽象一个市场数据源，便于后续切换实现（东财、新浪、Tushare 等）。
type Source interface {
	// ListAll 拉取全市场（沪深主板 / 创业板 / 科创板）股票清单。
	// 实现内部需要剔除 ST、退市、B 股、北交所及 ETF / 指数。
	ListAll(ctx context.Context) ([]model.Stock, error)

	// DailyKlines 拉取一只股票最近 n 根前复权日线。
	// 不足 n 根时按实际返回（如次新股）。
	DailyKlines(ctx context.Context, stock model.Stock, n int) ([]model.Kline, error)
}
