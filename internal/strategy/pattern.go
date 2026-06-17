package strategy

import "github.com/find-assets/scanner/internal/model"

// Pattern 表示一种"形态/命中逻辑"。它面对的是一段**已经是目标周期**的 K 线，
// 不关心这些 K 线是日线还是周线——周期转换由 Period 负责。
type Pattern interface {
	Name() string  // 形态标识，例如 "pierce" / "reversal"
	Label() string // 中文名，例如 "一箭穿心" / "超跌拐点"
	Eval(stock model.Stock, bars []model.Kline) (model.Result, bool)
}

// Info 描述一个可选项（周期或形态）的标识与中文名，供 CLI / API 列举。
type Info struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}
