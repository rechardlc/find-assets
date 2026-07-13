package crypto

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/find-assets/scanner/internal/crypto/pierce"
	"github.com/find-assets/scanner/internal/crypto/reversal"
	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/model"
)

// 支持的数字货币策略标识。
const (
	StrategyReversal = "reversal"
	StrategyPierce   = "pierce"
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

// RunReversal 仅运行拐点策略，返回单个报告（保持向后兼容）。
// 当周期 K 线根数不足需跳过时返回 (nil, nil)。
func (s *Service) RunReversal(ctx context.Context, job ScanJob) (*exporter.Report, error) {
	reps, err := s.RunScan(ctx, job, []string{StrategyReversal})
	if err != nil {
		return nil, err
	}
	return reps[StrategyReversal], nil
}

// activeStrategy 描述一次扫描中启用的某个策略及其判定入口。
type activeStrategy struct {
	name        string
	perAssetMin int // 单合约参与判定所需的最少 K 线根数
	evalAsset   func(model.Stock, []model.Kline) []model.Result
	pattern     string
	mode        string
	title       string
}

// RunScan 对同一周期只拉取一次 K 线，并发运行请求的多种策略；
// 每个策略产出独立报告（key 为策略名）。因 K 线仅拉取一次而非每策略一次，
// 多策略同周期扫描可省去重复请求。
func (s *Service) RunScan(ctx context.Context, job ScanJob, strategies []string) (map[string]*exporter.Report, error) {
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
	if len(strategies) == 0 {
		return map[string]*exporter.Report{}, nil
	}

	active, err := s.buildActiveStrategies(job, strategies)
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return map[string]*exporter.Report{}, nil
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

	resultsByStrat := s.scan(ctx, assets, job, active)

	finishedAt := time.Now()
	out := make(map[string]*exporter.Report, len(active))
	for _, st := range active {
		results := resultsByStrat[st.name]
		sort.Slice(results, func(i, j int) bool {
			if results[i].Code == results[j].Code {
				return results[i].Tag < results[j].Tag
			}
			return results[i].Code < results[j].Code
		})
		out[st.name] = &exporter.Report{
			AssetClass: exporter.AssetCrypto,
			Period:     job.Interval,
			Pattern:    st.pattern,
			Mode:       st.mode,
			Title:      st.title,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Elapsed:    finishedAt.Sub(startedAt).Round(10 * time.Millisecond).String(),
			Total:      len(assets),
			Matched:    len(results),
			Results:    results,
		}
	}
	return out, nil
}

func (s *Service) buildActiveStrategies(job ScanJob, strategies []string) ([]activeStrategy, error) {
	var active []activeStrategy
	for _, name := range strategies {
		switch name {
		case StrategyReversal:
			opt := reversal.DefaultOptions(job.Interval)
			minBars := reversal.MinRequiredBars(opt)
			if SkipsOnInsufficientBars(job.Interval) {
				if job.BarsLimit < minBars {
					continue // 1h/4h 根数不足：跳过该策略（不报错）
				}
			} else if job.BarsLimit < opt.OldBars {
				return nil, fmt.Errorf("bars %d 不足以计算 EMA120（至少需要 %d 根）", job.BarsLimit, opt.OldBars)
			}
			active = append(active, activeStrategy{
				name:        StrategyReversal,
				perAssetMin: minBars,
				evalAsset:   func(st model.Stock, ks []model.Kline) []model.Result { return evalReversal(st, ks, opt) },
				pattern:     "reversal",
				mode:        job.Interval + ":reversal",
				title:       IntervalTitle(job.Interval) + "拐点",
			})
		case StrategyPierce:
			opt := pierce.DefaultOptions(job.Interval)
			minBars := pierce.MinRequiredBars(opt)
			if job.BarsLimit < minBars {
				continue // 根数不足：跳过一箭穿心
			}
			active = append(active, activeStrategy{
				name:        StrategyPierce,
				perAssetMin: minBars,
				evalAsset:   func(st model.Stock, ks []model.Kline) []model.Result { return evalPierce(st, ks, opt) },
				pattern:     "pierce",
				mode:        job.Interval + ":pierce",
				title:       IntervalTitle(job.Interval) + "一箭穿心",
			})
		default:
			return nil, fmt.Errorf("未知数字货币策略: %s", name)
		}
	}
	return active, nil
}

func (s *Service) scan(ctx context.Context, assets []Asset, job ScanJob, active []activeStrategy) map[string][]model.Result {
	sem := make(chan struct{}, job.Workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	resultsByStrat := make(map[string][]model.Result, len(active))

scanLoop:
	for _, asset := range assets {
		select {
		case <-ctx.Done():
			break scanLoop
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
			stock := model.Stock{Code: asset.Symbol, Name: asset.Name}
			for _, st := range active {
				if len(klines) < st.perAssetMin {
					continue
				}
				rs := st.evalAsset(stock, klines)
				if len(rs) == 0 {
					continue
				}
				mu.Lock()
				resultsByStrat[st.name] = append(resultsByStrat[st.name], rs...)
				mu.Unlock()
			}
		}(asset)
	}

	wg.Wait()
	return resultsByStrat
}

func evalReversal(stock model.Stock, klines []model.Kline, opt reversal.Options) []model.Result {
	out := make([]model.Result, 0, 2)
	for _, dir := range []reversal.Direction{reversal.Oversold, reversal.Overbought} {
		if r, ok := reversal.Eval(stock, klines, dir, opt); ok {
			out = append(out, r)
		}
	}
	return out
}

func evalPierce(stock model.Stock, klines []model.Kline, opt pierce.Options) []model.Result {
	out := make([]model.Result, 0, 2)
	for _, dir := range []pierce.Direction{pierce.Up, pierce.Down} {
		if r, ok := pierce.Eval(stock, klines, dir, opt); ok {
			out = append(out, r)
		}
	}
	return out
}
