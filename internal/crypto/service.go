package crypto

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/find-assets/scanner/internal/crypto/reversal"
	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/model"
)

type Service struct {
	src Source
}

type ScanJob struct {
	Interval  string
	BarsLimit int
	Workers   int
	Assets    []Asset
}

func NewService(src Source) *Service {
	return &Service{src: src}
}

func (s *Service) RunReversal(ctx context.Context, job ScanJob) (*exporter.Report, error) {
	if s.src == nil {
		return nil, errors.New("数字货币数据源未配置")
	}
	if job.Interval == "" {
		return nil, errors.New("周期不能为空")
	}
	if _, err := resolveInterval(job.Interval); err != nil {
		return nil, err
	}
	if job.BarsLimit <= 0 {
		job.BarsLimit = 300
	}
	if job.Workers <= 0 {
		job.Workers = 10
	}

	opt := reversal.DefaultOptions(job.Interval)
	minBars := reversal.MinRequiredBars(opt)
	if SkipsOnInsufficientBars(job.Interval) {
		if job.BarsLimit < minBars {
			return nil, nil
		}
	} else if job.BarsLimit < opt.OldBars {
		return nil, fmt.Errorf("bars %d 不足以计算 EMA120（至少需要 %d 根）", job.BarsLimit, opt.OldBars)
	}

	startedAt := time.Now()
	assets := job.Assets
	if len(assets) == 0 {
		var err error
		assets, err = s.src.ListAssets(ctx)
		if err != nil {
			return nil, err
		}
	}

	results := s.scanReversal(ctx, assets, job, opt, minBars)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Code == results[j].Code {
			return results[i].Tag < results[j].Tag
		}
		return results[i].Code < results[j].Code
	})

	finishedAt := time.Now()
	return &exporter.Report{
		AssetClass: exporter.AssetCrypto,
		Period:     job.Interval,
		Pattern:    "reversal",
		Mode:       job.Interval + ":reversal",
		Title:      IntervalTitle(job.Interval) + "拐点",
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Elapsed:    finishedAt.Sub(startedAt).Round(10 * time.Millisecond).String(),
		Total:      len(assets),
		Matched:    len(results),
		Results:    results,
	}, nil
}

func (s *Service) scanReversal(ctx context.Context, assets []Asset, job ScanJob, opt reversal.Options, minBars int) []model.Result {
	sem := make(chan struct{}, job.Workers)
	out := make(chan model.Result, len(assets)*2)
	var wg sync.WaitGroup

	directions := []reversal.Direction{reversal.Oversold, reversal.Overbought}

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

			klines, err := s.src.Klines(ctx, asset, job.Interval, job.BarsLimit)
			if err != nil || len(klines) == 0 {
				return
			}
			if SkipsOnInsufficientBars(job.Interval) && len(klines) < minBars {
				return
			}
			stock := model.Stock{Code: asset.Symbol, Name: asset.Name}
			for _, dir := range directions {
				if r, ok := reversal.Eval(stock, klines, dir, opt); ok {
					out <- r
				}
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
