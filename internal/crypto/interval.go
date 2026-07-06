package crypto

import (
	"fmt"
	"strings"
	"time"
)

// IntervalSpec 描述一个可扫描的 K 线周期。
type IntervalSpec struct {
	Name     string
	Duration time.Duration
}

var DefaultIntervals = []IntervalSpec{
	{Name: "15m", Duration: 15 * time.Minute},
	{Name: "1h", Duration: time.Hour},
	{Name: "4h", Duration: 4 * time.Hour},
}

// ParseIntervalList 解析逗号分隔的周期列表，例如 "15m,1h,4h"。
func ParseIntervalList(raw string) ([]IntervalSpec, error) {
	parts := strings.Split(raw, ",")
	out := make([]IntervalSpec, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		spec, err := resolveInterval(name)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[spec.Name]; ok {
			continue
		}
		seen[spec.Name] = struct{}{}
		out = append(out, spec)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("至少需要一个有效周期（可选 15m | 1h | 4h）")
	}
	return out, nil
}

func resolveInterval(name string) (IntervalSpec, error) {
	switch name {
	case "15m":
		return IntervalSpec{Name: "15m", Duration: 15 * time.Minute}, nil
	case "1h":
		return IntervalSpec{Name: "1h", Duration: time.Hour}, nil
	case "4h":
		return IntervalSpec{Name: "4h", Duration: 4 * time.Hour}, nil
	default:
		return IntervalSpec{}, fmt.Errorf("未知周期: %q（可选 15m | 1h | 4h）", name)
	}
}

// MapIntervalForExchange 把 canonical 周期映射为交易所 API 参数。
func MapIntervalForExchange(exchange, canonical string) string {
	switch strings.ToLower(exchange) {
	case "okx":
		switch canonical {
		case "1h":
			return "1H"
		case "4h":
			return "4H"
		}
	}
	return canonical
}

// SkipsOnInsufficientBars 表示该周期在 K 线根数不足时跳过整轮扫描（不报错）。
func SkipsOnInsufficientBars(name string) bool {
	return name == "1h" || name == "4h"
}

// IntervalTitle 返回扫描报告用的中文周期标题。
func IntervalTitle(name string) string {
	switch name {
	case "15m":
		return "15分钟"
	case "1h":
		return "1小时"
	case "4h":
		return "4小时"
	default:
		return name
	}
}
