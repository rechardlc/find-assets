package model

// Stock 描述一只股票的基本信息。
type Stock struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Market 返回该股票所属的市场代号（用于拼接东方财富 secid）。
// 沪市（含科创板）返回 1，深市（含创业板）返回 0。
func (s Stock) Market() string {
	if len(s.Code) == 0 {
		return "1"
	}
	switch s.Code[0] {
	case '6':
		return "1"
	case '0', '3':
		return "0"
	default:
		return "1"
	}
}

// SecID 返回东方财富接口使用的 secid。
func (s Stock) SecID() string {
	return s.Market() + "." + s.Code
}
