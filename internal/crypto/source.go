package crypto

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/find-assets/scanner/internal/model"
)

type Source interface {
	Name() string
	ListAssets(ctx context.Context) ([]Asset, error)
	Klines(ctx context.Context, asset Asset, interval string, limit int) ([]model.Kline, error)
}

type CompositeSource struct {
	sources []Source
	active  int32
}

func NewCompositeSource(sources ...Source) (*CompositeSource, error) {
	if len(sources) == 0 {
		return nil, errors.New("至少需要一个数字货币数据源")
	}
	for _, src := range sources {
		if src == nil {
			return nil, errors.New("数字货币数据源不能为空")
		}
	}
	return &CompositeSource{sources: sources, active: -1}, nil
}

func (c *CompositeSource) Name() string {
	idx := int(atomic.LoadInt32(&c.active))
	if idx >= 0 && idx < len(c.sources) {
		return c.sources[idx].Name()
	}
	names := make([]string, 0, len(c.sources))
	for _, src := range c.sources {
		names = append(names, src.Name())
	}
	return strings.Join(names, ",")
}

func (c *CompositeSource) ListAssets(ctx context.Context) ([]Asset, error) {
	var errs []string
	for i, src := range c.sources {
		assets, err := src.ListAssets(ctx)
		if err == nil && len(assets) > 0 {
			atomic.StoreInt32(&c.active, int32(i))
			return assets, nil
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src.Name(), err))
		} else {
			errs = append(errs, fmt.Sprintf("%s: 空合约池", src.Name()))
		}
	}
	return nil, fmt.Errorf("所有数字货币数据源均失败: %s", strings.Join(errs, "; "))
}

func (c *CompositeSource) Klines(ctx context.Context, asset Asset, interval string, limit int) ([]model.Kline, error) {
	idx := int(atomic.LoadInt32(&c.active))
	order := make([]int, 0, len(c.sources))
	for i, src := range c.sources {
		if asset.Exchange != "" && src.Name() == asset.Exchange {
			order = append(order, i)
			break
		}
	}
	if idx >= 0 && idx < len(c.sources) {
		already := false
		for _, n := range order {
			if n == idx {
				already = true
				break
			}
		}
		if !already {
			order = append(order, idx)
		}
	}
	for i := range c.sources {
		if i != idx {
			already := false
			for _, n := range order {
				if n == i {
					already = true
					break
				}
			}
			if !already {
				order = append(order, i)
			}
		}
	}

	var lastErr error
	for _, i := range order {
		ks, err := c.sources[i].Klines(ctx, asset, interval, limit)
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
