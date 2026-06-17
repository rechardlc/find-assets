package model

// Snapshot 命中股票在判定当根 K 线上的指标快照，便于核对。
type Snapshot struct {
	Date           string  `json:"date"`
	Close          float64 `json:"close"`
	EMA5           float64 `json:"ema5"`
	EMA10          float64 `json:"ema10"`
	EMA30          float64 `json:"ema30"`
	EMA60          float64 `json:"ema60"`
	EMA120         float64 `json:"ema120,omitempty"`
	Range          float64 `json:"range,omitempty"`           // 仅 pierce 形态：实际粘合度（百分比）
	Volume         int64   `json:"volume,omitempty"`          // 仅 pierce 形态：命中当根成交量
	PrevVolume     int64   `json:"prev_volume,omitempty"`     // 仅 pierce 形态：前一根成交量
	VolumeIncrease float64 `json:"volume_increase,omitempty"` // 仅 pierce 形态：实际放量幅度（百分比）
	Bars           int     `json:"bars,omitempty"`            // 仅 reversal 形态：参与计算的 K 线根数
}

// Result 一条命中记录。
type Result struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Tag      string   `json:"tag"`
	Metric   string   `json:"metric,omitempty"`
	Snapshot Snapshot `json:"snapshot"`
}
