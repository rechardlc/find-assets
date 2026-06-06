package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/model"
	"github.com/find-assets/scanner/internal/scanner"
	"github.com/find-assets/scanner/internal/source"
	"github.com/find-assets/scanner/internal/strategy"
)

// ScanService 编排一次完整的扫描任务，CLI 与 HTTP 共用此层。
type ScanService struct {
	src source.Source
}

// New 构造一个使用东方财富数据源的 ScanService。
func New(src source.Source) *ScanService {
	return &ScanService{src: src}
}

// Source 返回底层数据源，便于上层读取活动源等元信息。
func (s *ScanService) Source() source.Source {
	return s.src
}

// Params 扫描参数。
type Params struct {
	Mode      string
	Workers   int
	BarsLimit int
	// Range 日线策略均线粘合度阈值（百分比，例如 2 表示 2%）。
	Range float64
	// Volume 日线策略放量阈值（百分比，例如 10 表示较前一日增加 10%）。
	Volume   float64
	TaskID   string
	Progress scanner.ProgressFn
	OnStocks func(total int, stocks []model.Stock) // 拉取到清单时回调
}

// Run 执行扫描并返回标准化报告。
func (s *ScanService) Run(ctx context.Context, p Params) (*exporter.Report, error) {
	strat := strategy.Get(p.Mode)
	if strat == nil {
		return nil, errors.New("未知策略模式: " + p.Mode)
	}
	if d, ok := strat.(*strategy.Day); ok && p.Range > 0 {
		d.Range = p.Range
	}
	if d, ok := strat.(*strategy.Day); ok && p.Volume > 0 {
		d.Volume = p.Volume
	}

	startedAt := time.Now()
	stocks, err := s.src.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("拉取股票清单失败: %w", err)
	}
	if p.OnStocks != nil {
		p.OnStocks(len(stocks), stocks)
	}

	results := scanner.Run(ctx, s.src, strat, stocks, scanner.Options{
		Workers:   p.Workers,
		BarsLimit: p.BarsLimit,
		Progress:  p.Progress,
	})
	finishedAt := time.Now()

	// day 策略：按均线粘合度从小到大排序（越小越粘合，越靠前）。
	if strat.Mode() == "day" {
		sort.SliceStable(results, func(i, j int) bool {
			return results[i].Snapshot.Range < results[j].Snapshot.Range
		})
	}

	rep := &exporter.Report{
		TaskID:     p.TaskID,
		Mode:       strat.Mode(),
		Title:      strat.Title(),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Elapsed:    finishedAt.Sub(startedAt).Round(10 * time.Millisecond).String(),
		Total:      len(stocks),
		Matched:    len(results),
		Results:    results,
	}
	return rep, nil
}
