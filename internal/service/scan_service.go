package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/find-assets/scanner/internal/exporter"
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

// Params 扫描参数。
type Params struct {
	Mode      string
	Workers   int
	BarsLimit int
	Cohesion  float64 // 仅对 day 策略生效，<=0 表示用默认值
	TaskID    string
	Progress  scanner.ProgressFn
	OnStocks  func(total int) // 拉取到清单时回调
}

// Run 执行扫描并返回标准化报告。
func (s *ScanService) Run(ctx context.Context, p Params) (*exporter.Report, error) {
	strat := strategy.Get(p.Mode)
	if strat == nil {
		return nil, errors.New("未知策略模式: " + p.Mode)
	}
	if d, ok := strat.(*strategy.Day); ok && p.Cohesion > 0 {
		d.Cohesion = p.Cohesion
	}

	startedAt := time.Now()
	stocks, err := s.src.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("拉取股票清单失败: %w", err)
	}
	if p.OnStocks != nil {
		p.OnStocks(len(stocks))
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
