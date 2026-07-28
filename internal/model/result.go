package model

// Snapshot 命中股票在判定当根 K 线上的指标快照，便于核对。
type Snapshot struct {
	Date           string  `json:"date"`
	Close          float64 `json:"close"`
	EMA5           float64 `json:"ema5,omitempty"`
	EMA10          float64 `json:"ema10,omitempty"`
	EMA30          float64 `json:"ema30,omitempty"`
	EMA60          float64 `json:"ema60,omitempty"`
	EMA120         float64 `json:"ema120,omitempty"`
	High           float64 `json:"high,omitempty"`            // amplitude 形态：命中当根最高价；box 形态：箱体上沿
	Low            float64 `json:"low,omitempty"`             // amplitude 形态：命中当根最低价；box 形态：箱体下沿
	Amplitude      float64 `json:"amplitude,omitempty"`       // 仅 amplitude 形态：实际振幅（百分比）
	Range          float64 `json:"range,omitempty"`           // pierce 形态：实际粘合度（百分比）；box 形态：箱体带宽（百分比）
	Volume         int64   `json:"volume,omitempty"`          // 仅 pierce 形态：命中当根成交量
	PrevVolume     int64   `json:"prev_volume,omitempty"`     // 仅 pierce 形态：前一根成交量
	VolumeIncrease float64 `json:"volume_increase,omitempty"` // 仅 pierce 形态：实际放量幅度（百分比）
	Touches        int     `json:"touches,omitempty"`         // 仅 box 形态：箱体被触及次数
	Bars           int     `json:"bars,omitempty"`            // 仅 reversal 形态：参与计算的 K 线根数
}

// Result 一条命中记录。
type Result struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Tag      string   `json:"tag"`
	Metric   string   `json:"metric,omitempty"`
	Alert    bool     `json:"alert,omitempty"` // 特殊标记（如数字货币一箭穿心的强多头信号），用于邮件高亮
	Snapshot Snapshot `json:"snapshot"`
}
