package crypto

import "testing"

func TestBuildHotAltPoolFiltersMajorsAndSortsByScore(t *testing.T) {
	metrics := []Metric{
		{Symbol: "BTCUSDT", ExchangeSymbol: "BTCUSDT", Base: "BTC", Quote: "USDT", Status: "TRADING", PriceChangePercent: 20, High24h: 120, Low24h: 80, Open24h: 100, QuoteVolume: 900000000, FundingRate: 0.001},
		{Symbol: "DOGEUSDT", ExchangeSymbol: "DOGEUSDT", Base: "DOGE", Quote: "USDT", Status: "TRADING", PriceChangePercent: -12, High24h: 1.3, Low24h: 0.9, Open24h: 1, QuoteVolume: 200000000, FundingRate: -0.0005},
		{Symbol: "PEPEUSDT", ExchangeSymbol: "PEPEUSDT", Base: "PEPE", Quote: "USDT", Status: "TRADING", PriceChangePercent: -18, High24h: 0.000013, Low24h: 0.000008, Open24h: 0.00001, QuoteVolume: 500000000, FundingRate: -0.0012},
		{Symbol: "HALTUSDT", ExchangeSymbol: "HALTUSDT", Base: "HALT", Quote: "USDT", Status: "BREAK", PriceChangePercent: -30, High24h: 2, Low24h: 1, Open24h: 1.5, QuoteVolume: 700000000, FundingRate: -0.002},
	}

	got := BuildHotAltPool(metrics, PoolOptions{Top: 2, ExcludeMajors: true})
	if len(got) != 2 {
		t.Fatalf("expected 2 assets, got %d: %+v", len(got), got)
	}
	if got[0].Symbol != "PEPEUSDT" {
		t.Fatalf("expected PEPE first, got %+v", got[0])
	}
	if got[1].Symbol != "DOGEUSDT" {
		t.Fatalf("expected DOGE second, got %+v", got[1])
	}
}
