package source

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/find-assets/scanner/internal/model"
)

// namedSource 是一个带名字的 Source，便于在日志中提示当前使用的数据源。
type namedSource struct {
	name string
	src  Source
}

// Composite 按声明顺序依次尝试多个数据源；前一个失败则切换到后一个。
// ListAll 失败时整体回退；DailyKlines 在调用阶段也会自动切换到下一个源。
type Composite struct {
	sources    []namedSource
	listActive int32 // 缓存 ListAll 成功的源下标，DailyKlines 优先复用
}

// NewComposite 构造组合数据源。常用：
//
//	auto:    em -> sina -> tencent
//	em:      em
//	sina:    sina
//	tencent: tencent (清单仍回退到新浪)
//	file:./stocks.json,em: 优先从本地文件读清单，K 线走东财
func NewComposite(spec string) (*Composite, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(spec)), ",")
	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		parts = []string{"auto"}
	}
	if len(parts) == 1 && parts[0] == "auto" {
		parts = []string{"em", "sina", "tencent"}
	}
	out := make([]namedSource, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case p == "em" || p == "eastmoney" || p == "":
			out = append(out, namedSource{name: "eastmoney", src: NewEastMoney()})
		case p == "sina":
			out = append(out, namedSource{name: "sina", src: NewSina()})
		case p == "tencent" || p == "tx":
			out = append(out, namedSource{name: "tencent", src: NewTencent()})
		case strings.HasPrefix(p, "file:"):
			path := strings.TrimPrefix(p, "file:")
			if path == "" {
				return nil, errors.New("file 源缺少路径，应写作 file:./stocks.json")
			}
			out = append(out, namedSource{name: "file:" + path, src: NewFileList(path)})
		default:
			return nil, fmt.Errorf("未知数据源: %s", p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("空的数据源配置")
	}
	return &Composite{sources: out}, nil
}

// ActiveName 返回当前用于扫描的数据源名（ListAll 成功后会被设置）。
func (c *Composite) ActiveName() string {
	idx := int(atomic.LoadInt32(&c.listActive))
	if idx < 0 || idx >= len(c.sources) {
		return ""
	}
	return c.sources[idx].name
}

// ListAll 依次尝试每个数据源。
func (c *Composite) ListAll(ctx context.Context) ([]model.Stock, error) {
	var errs []string
	for i, ns := range c.sources {
		stocks, err := ns.src.ListAll(ctx)
		if err == nil && len(stocks) > 0 {
			atomic.StoreInt32(&c.listActive, int32(i))
			return stocks, nil
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", ns.name, err))
		} else {
			errs = append(errs, fmt.Sprintf("%s: 空清单", ns.name))
		}
	}
	return nil, fmt.Errorf("所有数据源均失败: %s", strings.Join(errs, "; "))
}

// DailyKlines 优先用 ListAll 成功的源；失败时再尝试其他源。
func (c *Composite) DailyKlines(ctx context.Context, stock model.Stock, n int) ([]model.Kline, error) {
	idx := int(atomic.LoadInt32(&c.listActive))
	order := make([]int, 0, len(c.sources))
	if idx >= 0 && idx < len(c.sources) {
		order = append(order, idx)
	}
	for i := range c.sources {
		if i != idx {
			order = append(order, i)
		}
	}

	var lastErr error
	for _, i := range order {
		ks, err := c.sources[i].src.DailyKlines(ctx, stock, n)
		if err == nil {
			return ks, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("unknown error")
	}
	return nil, lastErr
}
