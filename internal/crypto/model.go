package crypto

import "time"

// Asset 是数字货币合约在系统内的统一表示。
type Asset struct {
	Symbol         string  `json:"symbol"`
	ExchangeSymbol string  `json:"exchange_symbol"`
	Name           string  `json:"name"`
	Base           string  `json:"base"`
	Quote          string  `json:"quote"`
	Exchange       string  `json:"exchange"`
	Score          float64 `json:"score,omitempty"`
}

// Metric 描述构建热门合约池所需的公开行情指标。
type Metric struct {
	Symbol             string
	ExchangeSymbol     string
	Base               string
	Quote              string
	Exchange           string
	Status             string
	PriceChangePercent float64
	High24h            float64
	Low24h             float64
	Open24h            float64
	QuoteVolume        float64
	FundingRate        float64
}

type PoolOptions struct {
	Top           int
	ExcludeMajors bool
}

type PoolCache struct {
	Date        string    `json:"date"`
	Exchange    string    `json:"exchange"`
	Pool        string    `json:"pool"`
	Contract    string    `json:"contract"`
	Top         int       `json:"top"`
	GeneratedAt time.Time `json:"generated_at"`
	Assets      []Asset   `json:"assets"`
}
