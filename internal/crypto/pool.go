package crypto

import (
	"math"
	"sort"
	"strings"
)

var majorBases = map[string]bool{
	"BTC": true,
	"ETH": true,
}

var stableBases = map[string]bool{
	"USDC":  true,
	"FDUSD": true,
	"TUSD":  true,
	"USDP":  true,
	"DAI":   true,
}

func BuildHotAltPool(metrics []Metric, opt PoolOptions) []Asset {
	candidates := make([]Asset, 0, len(metrics))
	for _, m := range metrics {
		if !eligibleMetric(m, opt) {
			continue
		}
		score := hotScore(m)
		candidates = append(candidates, Asset{
			Symbol:         m.Symbol,
			ExchangeSymbol: m.ExchangeSymbol,
			Name:           m.Base + " " + m.Quote + " Perpetual",
			Base:           m.Base,
			Quote:          m.Quote,
			Exchange:       m.Exchange,
			Score:          score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Symbol < candidates[j].Symbol
		}
		return candidates[i].Score > candidates[j].Score
	})
	if opt.Top > 0 && len(candidates) > opt.Top {
		candidates = candidates[:opt.Top]
	}
	return candidates
}

func eligibleMetric(m Metric, opt PoolOptions) bool {
	if strings.ToUpper(m.Quote) != "USDT" {
		return false
	}
	if strings.ToUpper(m.Status) != "TRADING" {
		return false
	}
	base := strings.ToUpper(m.Base)
	if stableBases[base] {
		return false
	}
	if opt.ExcludeMajors && majorBases[base] {
		return false
	}
	return true
}

func hotScore(m Metric) float64 {
	return Amplitude(m) +
		math.Abs(m.PriceChangePercent) +
		math.Log10(math.Max(m.QuoteVolume, 1)) +
		math.Abs(m.FundingRate)*10000
}

func Amplitude(m Metric) float64 {
	if m.Open24h <= 0 {
		return 0
	}
	return (m.High24h - m.Low24h) / m.Open24h * 100
}
