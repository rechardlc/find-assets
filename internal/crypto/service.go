package crypto

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/model"
	"github.com/find-assets/scanner/internal/strategy"
)

type Service struct {
	src Source
}

type Params struct {
	Interval  string
	Pattern   string
	BarsLimit int
	Workers   int
	Assets    []Asset
}

func NewService(src Source) *Service {
	return &Service{src: src}
}

func (s *Service) Run(ctx context.Context, p Params) (*exporter.Report, error) {
	if s.src == nil {
		return nil, errors.New("数字货币数据源未配置")
	}
	if p.Interval == "" {
		p.Interval = "15m"
	}
	if p.Pattern == "" {
		p.Pattern = "reversal"
	}
	if p.BarsLimit <= 0 {
		p.BarsLimit = 300
	}
	if p.Workers <= 0 {
		p.Workers = 10
	}

	strat, err := strategy.Get(p.Interval, p.Pattern, strategy.Options{})
	if err != nil {
		return nil, err
	}

	startedAt := time.Now()
	assets := p.Assets
	if len(assets) == 0 {
		assets, err = s.src.ListAssets(ctx)
		if err != nil {
			return nil, err
		}
	}

	results := s.scan(ctx, strat, assets, p.Interval, p.BarsLimit, p.Workers)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Code < results[j].Code
	})

	finishedAt := time.Now()
	return &exporter.Report{
		Period:     strat.Period(),
		Pattern:    strat.Pattern(),
		Mode:       strat.Mode(),
		Title:      strat.Title(),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Elapsed:    finishedAt.Sub(startedAt).Round(10 * time.Millisecond).String(),
		Total:      len(assets),
		Matched:    len(results),
		Results:    results,
	}, nil
}

func (s *Service) scan(ctx context.Context, strat strategy.Strategy, assets []Asset, interval string, barsLimit, workers int) []model.Result {
	sem := make(chan struct{}, workers)
	out := make(chan model.Result, len(assets))
	var wg sync.WaitGroup

	for _, asset := range assets {
		select {
		case <-ctx.Done():
			break
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(asset Asset) {
			defer wg.Done()
			defer func() { <-sem }()

			klines, err := s.src.Klines(ctx, asset, interval, barsLimit)
			if err != nil || len(klines) == 0 {
				return
			}
			stock := model.Stock{Code: asset.Symbol, Name: asset.Name}
			if r, ok := strat.Match(stock, klines); ok {
				out <- r
			}
		}(asset)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	results := make([]model.Result, 0)
	for r := range out {
		results = append(results, r)
	}
	return results
}
