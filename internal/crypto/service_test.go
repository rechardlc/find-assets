package crypto

import (
	"context"
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

type fakeSource struct {
	assets []Asset
	klines []model.Kline
}

func (f fakeSource) Name() string { return "fake" }

func (f fakeSource) ListAssets(context.Context) ([]Asset, error) {
	return f.assets, nil
}

func (f fakeSource) Klines(context.Context, Asset, string, int) ([]model.Kline, error) {
	return f.klines, nil
}

func TestServiceRunBuildsReport(t *testing.T) {
	src := fakeSource{
		assets: []Asset{
			{Symbol: "PEPEUSDT", ExchangeSymbol: "PEPEUSDT", Base: "PEPE", Quote: "USDT", Exchange: "fake"},
			{Symbol: "DOGEUSDT", ExchangeSymbol: "DOGEUSDT", Base: "DOGE", Quote: "USDT", Exchange: "fake"},
		},
		klines: makeFlatKlines(300),
	}

	rep, err := NewService(src).Run(context.Background(), Params{
		Interval:  "15m",
		Pattern:   "reversal",
		BarsLimit: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Period != "15m" || rep.Pattern != "reversal" || rep.Mode != "15m:reversal" {
		t.Fatalf("unexpected strategy fields: %+v", rep)
	}
	if rep.Total != 2 {
		t.Fatalf("expected total 2, got %d", rep.Total)
	}
}

func makeFlatKlines(n int) []model.Kline {
	out := make([]model.Kline, n)
	start := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	for i := range out {
		out[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * 15 * time.Minute),
			Open:   1,
			Close:  1,
			High:   1,
			Low:    1,
			Volume: 1000,
		}
	}
	return out
}
