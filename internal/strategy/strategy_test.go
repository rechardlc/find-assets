package strategy

import (
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

// 构造 N 天的等值收盘价 + 最后一天一根穿心阳线，验证 Day 策略命中。
func TestDay_Match_HitArrowThroughHeart(t *testing.T) {
	d := NewDay()
	d.Range = 5 // 放宽阈值至 5%，便于构造

	// 130 天的价格全部贴在 10.0 附近，让 5 条 EMA 高度粘合
	const days = 130
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	daily := make([]model.Kline, 0, days+1)
	for i := 0; i < days; i++ {
		daily = append(daily, model.Kline{
			Date:   start.AddDate(0, 0, i),
			Open:   10,
			Close:  10,
			High:   10.05,
			Low:    9.95,
			Volume: 1000,
		})
	}
	// 最后一天画一根穿心阳线：开盘 9.5，最高 11.0，最低 9.0，收盘 10.8
	daily = append(daily, model.Kline{
		Date:   start.AddDate(0, 0, days),
		Open:   9.5,
		Close:  10.8,
		High:   11.0,
		Low:    9.0,
		Volume: 1200,
	})

	stk := model.Stock{Code: "600000", Name: "示例银行"}
	r, ok := d.Match(stk, daily)
	if !ok {
		t.Fatalf("expected match")
	}
	if r.Code != "600000" || r.Snapshot.Range < 0 {
		t.Fatalf("unexpected result: %+v", r)
	}
}

// 一箭穿心当天必须放量，默认至少比前一日成交量高 10%。
func TestDay_Match_RequiresVolumeIncrease(t *testing.T) {
	d := NewDay()
	d.Range = 5 // 放宽阈值至 5%，便于构造

	const days = 130
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	daily := make([]model.Kline, 0, days+1)
	for i := 0; i < days; i++ {
		daily = append(daily, model.Kline{
			Date:   start.AddDate(0, 0, i),
			Open:   10,
			Close:  10,
			High:   10.05,
			Low:    9.95,
			Volume: 1000,
		})
	}
	daily = append(daily, model.Kline{
		Date:   start.AddDate(0, 0, days),
		Open:   9.5,
		Close:  10.8,
		High:   11.0,
		Low:    9.0,
		Volume: 1099,
	})

	if _, ok := d.Match(model.Stock{Code: "600000", Name: "示例银行"}, daily); ok {
		t.Fatal("should not match when arrow-through-heart day is not 10% above previous volume")
	}
}

// 上市不足 125 天的次新股应被淘汰。
func TestDay_Match_NotEnoughBars(t *testing.T) {
	d := NewDay()
	daily := make([]model.Kline, 120)
	for i := range daily {
		daily[i] = model.Kline{
			Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open: 10, Close: 10, High: 10, Low: 10,
		}
	}
	if _, ok := d.Match(model.Stock{Code: "600000"}, daily); ok {
		t.Fatal("should not match when bars < 125")
	}
}

// 长期单边下跌虽呈空头排列，但 EMA30/EMA60 不会交织，应不命中。
func TestWeek_Match_SteadyDeclineNoCross(t *testing.T) {
	w := NewWeek()
	const weeks = 80
	const barsPerWeek = 5
	daily := make([]model.Kline, 0, weeks*barsPerWeek)
	start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
	price := 100.0
	for wi := 0; wi < weeks; wi++ {
		for di := 0; di < barsPerWeek; di++ {
			daily = append(daily, model.Kline{
				Date:  start.AddDate(0, 0, wi*7+di),
				Open:  price,
				Close: price * 0.99,
				High:  price * 1.001,
				Low:   price * 0.985,
			})
			price *= 0.99
		}
	}
	if _, ok := w.Match(model.Stock{Code: "300999", Name: "示例新股"}, daily); ok {
		t.Fatal("steady decline without EMA cross should not match")
	}
}

// 上市不足 60 周直接淘汰
func TestWeek_Match_TooNew(t *testing.T) {
	w := NewWeek()
	daily := make([]model.Kline, 30*5) // ~30 周
	for i := range daily {
		daily[i] = model.Kline{
			Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open: 10, Close: 10, High: 10, Low: 10,
		}
	}
	if _, ok := w.Match(model.Stock{Code: "300999"}, daily); ok {
		t.Fatal("should not match when weeks < 60")
	}
}
