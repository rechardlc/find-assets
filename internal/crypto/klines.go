package crypto

import (
	"time"

	"github.com/find-assets/scanner/internal/model"
)

// DropFormingBar 去掉序列末尾仍在形成中的 K 线。
// OKX /candles 会把当前周期未收盘 K 线作为最新一根返回；策略应基于上一根已收盘 K 线判定。
func DropFormingBar(bars []model.Kline, dur time.Duration, now time.Time) []model.Kline {
	if len(bars) == 0 || dur <= 0 {
		return bars
	}
	currentOpen := now.Truncate(dur)
	last := bars[len(bars)-1]
	if !last.Date.Before(currentOpen) {
		return bars[:len(bars)-1]
	}
	return bars
}
