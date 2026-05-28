package model

// Snapshot 命中股票在判定当根 K 线上的指标快照，便于核对。
type Snapshot struct {
	Date     string  `json:"date"`
	Close    float64 `json:"close"`
	EMA5     float64 `json:"ema5"`
	EMA10    float64 `json:"ema10"`
	EMA30    float64 `json:"ema30"`
	EMA60    float64 `json:"ema60"`
	EMA120   float64 `json:"ema120,omitempty"`
	Cohesion float64 `json:"cohesion,omitempty"` // 仅 day 策略
	Weeks    int     `json:"weeks,omitempty"`    // 仅 week 策略
}

// Result 一条命中记录。
type Result struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Tag      string   `json:"tag"`
	Metric   string   `json:"metric,omitempty"`
	Snapshot Snapshot `json:"snapshot"`
}
