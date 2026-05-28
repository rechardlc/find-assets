package model

import "time"

// Kline 表示一根 K 线（日线或周线通用）。
type Kline struct {
	Date   time.Time `json:"date"`
	Open   float64   `json:"open"`
	Close  float64   `json:"close"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Volume int64     `json:"volume"`
}

// Closes 抽取一组 K 线的收盘价，便于送入 EMA 计算。
func Closes(ks []Kline) []float64 {
	out := make([]float64, len(ks))
	for i, k := range ks {
		out[i] = k.Close
	}
	return out
}
