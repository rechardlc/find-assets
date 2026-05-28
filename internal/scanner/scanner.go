package scanner

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/find-assets/scanner/internal/model"
	"github.com/find-assets/scanner/internal/source"
	"github.com/find-assets/scanner/internal/strategy"
)

// ProgressFn 用于实时回调扫描进度。done 为已完成数，total 为总数。
type ProgressFn func(done, total int64)

// Options 扫描参数。
type Options struct {
	Workers   int // 最大并发，默认 100
	BarsLimit int // 拉取日线根数，默认 600
	Progress  ProgressFn
}

// Run 在 stocks 列表上并发执行指定策略，返回所有命中的结果。
func Run(
	ctx context.Context,
	src source.Source,
	strat strategy.Strategy,
	stocks []model.Stock,
	opt Options,
) []model.Result {
	if opt.Workers <= 0 {
		opt.Workers = 100
	}
	if opt.BarsLimit <= 0 {
		opt.BarsLimit = 600
	}

	total := int64(len(stocks))
	sem := make(chan struct{}, opt.Workers)
	var wg sync.WaitGroup
	var done int64

	out := make(chan model.Result, 256)

	for _, st := range stocks {
		select {
		case <-ctx.Done():
			break
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(st model.Stock) {
			defer wg.Done()
			defer func() { <-sem }()

			klines, err := src.DailyKlines(ctx, st, opt.BarsLimit)
			if err == nil && len(klines) > 0 {
				if r, ok := strat.Match(st, klines); ok {
					out <- r
				}
			}
			n := atomic.AddInt64(&done, 1)
			if opt.Progress != nil {
				opt.Progress(n, total)
			}
		}(st)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	results := make([]model.Result, 0, 64)
	for r := range out {
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Code < results[j].Code
	})
	return results
}
