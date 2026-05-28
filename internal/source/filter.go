package source

import (
	"strings"

	"github.com/find-assets/scanner/internal/model"
)

// IsTradable 判断给定股票是否属于扫描范围。
// 范围：沪深主板、创业板、科创板。
// 剔除：ST/*ST/退市、B 股、北交所、指数 / ETF。
func IsTradable(s model.Stock) bool {
	name := strings.ToUpper(s.Name)
	if name == "" {
		return false
	}
	if strings.Contains(name, "ST") || strings.Contains(name, "退") {
		return false
	}
	code := s.Code
	if len(code) != 6 {
		return false
	}
	switch {
	case strings.HasPrefix(code, "600"),
		strings.HasPrefix(code, "601"),
		strings.HasPrefix(code, "603"),
		strings.HasPrefix(code, "605"): // 沪市主板
		return true
	case strings.HasPrefix(code, "000"),
		strings.HasPrefix(code, "001"),
		strings.HasPrefix(code, "002"),
		strings.HasPrefix(code, "003"): // 深市主板（含原中小板）
		return true
	case strings.HasPrefix(code, "300"),
		strings.HasPrefix(code, "301"): // 创业板
		return true
	case strings.HasPrefix(code, "688"),
		strings.HasPrefix(code, "689"): // 科创板
		return true
	}
	return false
}
