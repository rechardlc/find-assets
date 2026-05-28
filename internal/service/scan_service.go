package service

import (
	"context"
	"errors"
	"fmt"
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
	// Range 日线策略均线粘合度阈值（百分比，例如 1.5 表示 1.5%）；> 0 时优先生效。
	Range float64
	// Cohesion 日线策略均线粘合度阈值（小数，例如 0.015 表示 1.5%）；
	// 仅当 Range <= 0 时生效，用于向后兼容。
	Cohesion float64
	TaskID   string
	Progress scanner.ProgressFn
	OnStocks func(total int, stocks []model.Stock) // 拉取到清单时回调
}

// effectiveCohesion 返回最终生效的粘合度阈值（小数形式）。
// Range > 0 时按百分比换算，否则回退到 Cohesion。
func (p Params) effectiveCohesion() float64 {
	if p.Range > 0 {
		return p.Range / 100
	}
	return p.Cohesion
}

// Run 执行扫描并返回标准化报告。
func (s *ScanService) Run(ctx context.Context, p Params) (*exporter.Report, error) {
	strat := strategy.Get(p.Mode)
	if strat == nil {
		return nil, errors.New("未知策略模式: " + p.Mode)
	}
	if d, ok := strat.(*strategy.Day); ok {
		if c := p.effectiveCohesion(); c > 0 {
			d.Cohesion = c
		}
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
