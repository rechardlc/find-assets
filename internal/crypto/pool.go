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

// hotAltWeights 是 hot_alt 综合热度四项指标的权重。
// 指标先在池内做 min-max 归一化再加权求和，避免量纲不一致导致某项失效。
var hotAltWeights = [4]float64{
	0.35, // amplitude 振幅
	0.25, // abs price change 涨跌幅绝对值
	0.30, // log10 quote volume 成交额
	0.10, // abs funding rate 资金费率绝对值
}

func BuildHotAltPool(metrics []Metric, opt PoolOptions) []Asset {
	type candidate struct {
		asset  Asset
		values [4]float64
	}
	rows := make([]candidate, 0, len(metrics))
	for _, m := range metrics {
		if !eligibleMetric(m, opt) {
			continue
		}
		rows = append(rows, candidate{
			asset: Asset{
				Symbol:         m.Symbol,
				ExchangeSymbol: m.ExchangeSymbol,
				Name:           m.Base + " " + m.Quote + " Perpetual",
				Base:           m.Base,
				Quote:          m.Quote,
				Exchange:       m.Exchange,
			},
			values: rawComponents(m),
		})
	}
	if len(rows) == 0 {
		return nil
	}

	var minv, maxv [4]float64
	for j := 0; j < 4; j++ {
		minv[j] = math.Inf(1)
		maxv[j] = math.Inf(-1)
	}
	for _, r := range rows {
		for j := 0; j < 4; j++ {
			minv[j] = math.Min(minv[j], r.values[j])
			maxv[j] = math.Max(maxv[j], r.values[j])
		}
	}

	// 只有存在差异（max>min）的列才参与打分；恒定列（如 OKX 暂缺资金费率恒为 0）
	// 其权重按比例重分配给其余活跃列，避免缺项被当 0 拉低整体分数。
	var totalActiveWeight float64
	var active [4]bool
	for j := 0; j < 4; j++ {
		if maxv[j] > minv[j] {
			active[j] = true
			totalActiveWeight += hotAltWeights[j]
		}
	}

	candidates := make([]Asset, 0, len(rows))
	for _, r := range rows {
		var score float64
		if totalActiveWeight > 0 {
			for j := 0; j < 4; j++ {
				if !active[j] {
					continue
				}
				norm := (r.values[j] - minv[j]) / (maxv[j] - minv[j])
				score += hotAltWeights[j] / totalActiveWeight * norm
			}
			score *= 100
		}
		a := r.asset
		a.Score = math.Round(score*100) / 100
		candidates = append(candidates, a)
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

// rawComponents 返回一个合约的四项原始热度指标（未归一化）。
func rawComponents(m Metric) [4]float64 {
	return [4]float64{
		Amplitude(m),
		math.Abs(m.PriceChangePercent),
		math.Log10(math.Max(m.QuoteVolume, 1)),
		math.Abs(m.FundingRate),
	}
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

func Amplitude(m Metric) float64 {
	if m.Open24h <= 0 {
		return 0
	}
	return (m.High24h - m.Low24h) / m.Open24h * 100
}
