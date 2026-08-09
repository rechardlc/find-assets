package crypto

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

type fakeSource struct {
	assets []Asset
	klines []model.Kline
}

func (f fakeSource) Name() string { return "fake" }

func (f fakeSource) ListAssets(context.Context) ([]Asset, error) {
	return f.assets, nil
}

func (f fakeSource) Klines(context.Context, Asset, string, int) ([]model.Kline, error) {
	return f.klines, nil
}

// multiKlineSource 按交易对返回不同 K 线，用于排序等多资产场景。
type multiKlineSource struct {
	assets []Asset
	klines map[string][]model.Kline
}

func (m multiKlineSource) Name() string { return "fake" }

func (m multiKlineSource) ListAssets(context.Context) ([]Asset, error) {
	return m.assets, nil
}

func (m multiKlineSource) Klines(_ context.Context, asset Asset, _ string, _ int) ([]model.Kline, error) {
	return m.klines[asset.Symbol], nil
}

// multiIntervalSource 按 symbol|interval 返回 K 线，供 RunTrend 三周期测试。
type multiIntervalSource struct {
	assets []Asset
	klines map[string][]model.Kline
}

func (m multiIntervalSource) Name() string { return "fake" }

func (m multiIntervalSource) ListAssets(context.Context) ([]Asset, error) {
	return m.assets, nil
}

func (m multiIntervalSource) Klines(_ context.Context, asset Asset, interval string, _ int) ([]model.Kline, error) {
	ks, ok := m.klines[asset.Symbol+"|"+interval]
	if !ok {
		return nil, errors.New("no klines")
	}
	return ks, nil
}

func TestRunReversalSkipsLongIntervalWhenBarsInsufficient(t *testing.T) {
	src := fakeSource{
		assets: []Asset{{Symbol: "PEPEUSDT"}},
		klines: makeFlatKlines(300),
	}
	for _, interval := range []string{"1h", "4h"} {
		rep, err := NewService(src).RunReversal(context.Background(), ScanJob{
			Interval:  interval,
			BarsLimit: 100,
		})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", interval, err)
		}
		if rep != nil {
			t.Fatalf("%s: expected nil report when bars insufficient", interval)
		}
	}
}

func TestRunReversalSkipsLongIntervalAssetWithFewKlines(t *testing.T) {
	src := fakeSource{
		assets: []Asset{
			{Symbol: "NEWUSDT"},
			{Symbol: "OLDUSDT"},
		},
		klines: makeFlatKlines(80),
	}
	rep, err := NewService(src).RunReversal(context.Background(), ScanJob{
		Interval:  "1h",
		BarsLimit: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep == nil {
		t.Fatal("expected report when bars limit is sufficient")
	}
	if rep.Matched != 0 {
		t.Fatalf("expected no matches when klines below minimum, got %d", rep.Matched)
	}
}

func TestServiceRunBuildsReport(t *testing.T) {
	src := fakeSource{
		assets: []Asset{
			{Symbol: "PEPEUSDT", ExchangeSymbol: "PEPEUSDT", Base: "PEPE", Quote: "USDT", Exchange: "fake"},
			{Symbol: "DOGEUSDT", ExchangeSymbol: "DOGEUSDT", Base: "DOGE", Quote: "USDT", Exchange: "fake"},
		},
		klines: makeFlatKlines(300),
	}

	rep, err := NewService(src).RunReversal(context.Background(), ScanJob{
		Interval:  "15m",
		BarsLimit: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Period != "15m" || rep.Pattern != "reversal" || rep.Mode != "15m:reversal" {
		t.Fatalf("unexpected strategy fields: %+v", rep)
	}
	if rep.Title != "15分钟拐点" {
		t.Fatalf("unexpected title: %q", rep.Title)
	}
	if rep.Total != 2 {
		t.Fatalf("expected total 2, got %d", rep.Total)
	}
}

func TestRunScanRunsBothStrategies(t *testing.T) {
	src := fakeSource{
		assets: []Asset{{Symbol: "PEPEUSDT"}},
		klines: makeFlatKlines(300),
	}
	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "1h",
		BarsLimit: 300,
	}, []string{StrategyReversal, StrategyPierce})
	if err != nil {
		t.Fatal(err)
	}
	if reps[StrategyReversal] == nil || reps[StrategyPierce] == nil {
		t.Fatalf("expected both reports, got %+v", reps)
	}
	if reps[StrategyPierce].Pattern != "pierce" || reps[StrategyPierce].Mode != "1h:pierce" {
		t.Fatalf("unexpected pierce report: %+v", reps[StrategyPierce])
	}
	if reps[StrategyPierce].Title != "1小时一箭穿心" {
		t.Fatalf("unexpected pierce title: %q", reps[StrategyPierce].Title)
	}
}

func TestRunScanSkipsPierceWhenBarsInsufficient(t *testing.T) {
	src := fakeSource{
		assets: []Asset{{Symbol: "X"}},
		klines: makeFlatKlines(100),
	}
	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "1h",
		BarsLimit: 100,
	}, []string{StrategyPierce})
	if err != nil {
		t.Fatal(err)
	}
	if reps[StrategyPierce] != nil {
		t.Fatalf("expected pierce skipped when bars insufficient, got %+v", reps[StrategyPierce])
	}
}

func TestRunScanAmplitudeUsesInjectedThreshold(t *testing.T) {
	klines := makeFlatKlines(10)
	sig := len(klines) - 2
	klines[sig] = model.Kline{Date: klines[sig].Date, Open: 1, High: 1.1, Low: 1, Close: 1.09, Volume: 2000} // 振幅 10%

	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: klines}

	rep, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "4h",
		BarsLimit: 300,
	}, []string{StrategyAmplitude})
	if err != nil {
		t.Fatal(err)
	}
	amp := rep[StrategyAmplitude]
	if amp == nil {
		t.Fatal("expected amplitude report")
	}
	if amp.Pattern != "amplitude" || amp.Mode != "4h:amplitude" {
		t.Fatalf("unexpected amplitude report: %+v", amp)
	}
	if amp.Title != "4小时振幅异动(≥9%)" {
		t.Fatalf("unexpected amplitude title: %q", amp.Title)
	}
	if amp.Matched != 1 {
		t.Fatalf("expected 1 match at default 9%% threshold, got %d", amp.Matched)
	}

	raised, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:     "4h",
		BarsLimit:    300,
		AmplitudePct: 15,
	}, []string{StrategyAmplitude})
	if err != nil {
		t.Fatal(err)
	}
	if got := raised[StrategyAmplitude]; got == nil || got.Matched != 0 {
		t.Fatalf("expected no match when threshold raised to 15%%, got %+v", got)
	}
	if got := raised[StrategyAmplitude].Title; got != "4小时振幅异动(≥15%)" {
		t.Fatalf("unexpected title with injected threshold: %q", got)
	}
}

// 振幅只看上一根 K 线，不受根数不足影响（拐点/穿心会跳过的场景仍应产出报告）。
func TestRunScanAmplitudeRunsWithFewBars(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "X"}}, klines: makeFlatKlines(5)}
	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "4h",
		BarsLimit: 5,
	}, []string{StrategyAmplitude, StrategyPierce})
	if err != nil {
		t.Fatal(err)
	}
	if reps[StrategyAmplitude] == nil {
		t.Fatal("expected amplitude report even with few bars")
	}
	if reps[StrategyPierce] != nil {
		t.Fatalf("expected pierce skipped with few bars, got %+v", reps[StrategyPierce])
	}
}

func TestRunScanBoxBuildsReport(t *testing.T) {
	// 箱体序列：顶底同时命中时合并为一条窄幅横盘。
	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: makeBoxKlines(300)}

	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "4h",
		BarsLimit: 300,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	rep := reps[StrategyBox]
	if rep == nil {
		t.Fatal("expected box report")
	}
	if rep.Pattern != "box" || rep.Mode != "4h:box" {
		t.Fatalf("unexpected box report: %+v", rep)
	}
	if rep.Title != "4小时箱体震荡(带宽≤0.6%·振幅≥5%·触及≥3次·间隔≥6根)" {
		t.Fatalf("unexpected box title: %q", rep.Title)
	}
	if rep.Matched != 1 {
		t.Fatalf("expected sideways merge of bottom+top into 1 result, got %d", rep.Matched)
	}
	if !strings.Contains(rep.Results[0].Tag, "窄幅横盘") {
		t.Fatalf("expected 窄幅横盘 tag, got %s", rep.Results[0].Tag)
	}
}

// 强势（Alert）优先于代码序：字母序靠前的普通命中不得排在强势之前。
func TestRunScanSortsAlertBeforeNonAlert(t *testing.T) {
	weak := makeFlatKlines(10)
	sig := len(weak) - 2
	weak[sig] = model.Kline{Date: weak[sig].Date, Open: 101, High: 110, Low: 100, Close: 109, Volume: 2000} // 10%

	strong := makeFlatKlines(10)
	strong[sig] = model.Kline{Date: strong[sig].Date, Open: 100, High: 118, Low: 100, Close: 117, Volume: 2000} // 18%

	ms := multiKlineSource{
		assets: []Asset{{Symbol: "AAAUSDT"}, {Symbol: "ZZZUSDT"}},
		klines: map[string][]model.Kline{
			"AAAUSDT": weak,
			"ZZZUSDT": strong,
		},
	}
	reps, err := NewService(ms).RunScan(context.Background(), ScanJob{
		Interval:  "4h",
		BarsLimit: 300,
	}, []string{StrategyAmplitude})
	if err != nil {
		t.Fatal(err)
	}
	rep := reps[StrategyAmplitude]
	if rep == nil || rep.Matched != 2 {
		t.Fatalf("expected 2 amplitude hits, got %+v", rep)
	}
	if !rep.Results[0].Alert || rep.Results[1].Alert {
		t.Fatalf("expected Alert then non-Alert, got alert=%v/%v codes=%s/%s",
			rep.Results[0].Alert, rep.Results[1].Alert,
			rep.Results[0].Code, rep.Results[1].Code)
	}
	if rep.Results[0].Code != "ZZZUSDT" || rep.Results[1].Code != "AAAUSDT" {
		t.Fatalf("expected ZZZUSDT before AAAUSDT, got %s then %s",
			rep.Results[0].Code, rep.Results[1].Code)
	}
}

// 振幅报告在同档内按振幅降序：字母序靠前的普通命中不得排在振幅更大的之前。
func TestRunScanAmplitudeSortsByAmplitudeDescending(t *testing.T) {
	sig := func(bars []model.Kline, o, h, l, c float64) {
		i := len(bars) - 2
		bars[i] = model.Kline{Date: bars[i].Date, Open: o, High: h, Low: l, Close: c, Volume: 2000}
	}
	lowAmp := makeFlatKlines(10)
	sig(lowAmp, 100, 110, 100, 109) // ~10%
	highAmp := makeFlatKlines(10)
	sig(highAmp, 100, 115, 100, 114) // 15%

	ms := multiKlineSource{
		assets: []Asset{{Symbol: "AAAUSDT"}, {Symbol: "BBBUSDT"}},
		klines: map[string][]model.Kline{
			"AAAUSDT": lowAmp,
			"BBBUSDT": highAmp,
		},
	}
	reps, err := NewService(ms).RunScan(context.Background(), ScanJob{
		Interval:  "4h",
		BarsLimit: 300,
	}, []string{StrategyAmplitude})
	if err != nil {
		t.Fatal(err)
	}
	rep := reps[StrategyAmplitude]
	if rep == nil || rep.Matched != 2 {
		t.Fatalf("expected 2 amplitude hits, got %+v", rep)
	}
	if rep.Results[0].Code != "BBBUSDT" || rep.Results[1].Code != "AAAUSDT" {
		t.Fatalf("expected higher amplitude first, got %s then %s",
			rep.Results[0].Code, rep.Results[1].Code)
	}
	if rep.Results[0].Snapshot.Amplitude <= rep.Results[1].Snapshot.Amplitude {
		t.Fatalf("expected amplitude desc, got %v then %v",
			rep.Results[0].Snapshot.Amplitude, rep.Results[1].Snapshot.Amplitude)
	}
}

// BoxSidewaysOnly=true 时只保留顶底同时命中的窄幅横盘；false 时仅底/仅顶也保留。
func TestRunScanBoxSidewaysOnlyFiltersSingleSide(t *testing.T) {
	ms := multiKlineSource{
		assets: []Asset{{Symbol: "LOWUSDT"}, {Symbol: "BOTHUSDT"}},
		klines: map[string][]model.Kline{
			"LOWUSDT":  sparseBottomBox(30),
			"BOTHUSDT": makeBoxKlines(300),
		},
	}
	reps, err := NewService(ms).RunScan(context.Background(), ScanJob{
		Interval:        "4h",
		BarsLimit:       300,
		BoxSidewaysOnly: true,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	rep := reps[StrategyBox]
	if rep == nil || rep.Matched != 1 {
		t.Fatalf("expected only sideways hit, got %+v", rep)
	}
	if rep.Results[0].Code != "BOTHUSDT" || !strings.Contains(rep.Results[0].Tag, "窄幅横盘") {
		t.Fatalf("unexpected result: %+v", rep.Results[0])
	}

	all, err := NewService(ms).RunScan(context.Background(), ScanJob{
		Interval:        "4h",
		BarsLimit:       300,
		BoxSidewaysOnly: false,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	if got := all[StrategyBox]; got == nil || got.Matched != 2 {
		t.Fatalf("expected both single-side and sideways when flag false, got %+v", got)
	}
}

// 箱体报告按触及次数降序：触及更多的合约排在前面。
func TestRunScanBoxSortsByTouchesDescending(t *testing.T) {
	ms := multiKlineSource{
		assets: []Asset{{Symbol: "LOWUSDT"}, {Symbol: "HIGHUSDT"}},
		klines: map[string][]model.Kline{
			"LOWUSDT":  sparseBottomBox(30),
			"HIGHUSDT": makeBoxKlines(300),
		},
	}
	reps, err := NewService(ms).RunScan(context.Background(), ScanJob{
		Interval:  "4h",
		BarsLimit: 300,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	rep := reps[StrategyBox]
	if rep == nil || rep.Matched != 2 {
		t.Fatalf("expected 2 box hits for sort test, got %+v", rep)
	}
	if rep.Results[0].Snapshot.Touches < rep.Results[1].Snapshot.Touches {
		t.Fatalf("results not sorted by touches desc: %s=%d then %s=%d",
			rep.Results[0].Code, rep.Results[0].Snapshot.Touches,
			rep.Results[1].Code, rep.Results[1].Snapshot.Touches)
	}
	if rep.Results[0].Code != "HIGHUSDT" || rep.Results[1].Code != "LOWUSDT" {
		t.Fatalf("expected HIGHUSDT before LOWUSDT, got %s then %s", rep.Results[0].Code, rep.Results[1].Code)
	}
}

func TestRunScanBoxUsesInjectedOptions(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: makeBoxKlines(300)}

	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:        "1h",
		BarsLimit:       300,
		BoxPct:          1.2,
		BoxTouches:      4,
		BoxMinGap:       10,
		BoxAmplitudePct: 3,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	if got := reps[StrategyBox]; got == nil || got.Title != "1小时箱体震荡(带宽≤1.2%·振幅≥3%·触及≥4次·间隔≥10根)" {
		t.Fatalf("injected box options not reflected in report: %+v", got)
	}
}

// 死水横盘：箱体本身成立，但跨度内振幅为 0，新增门槛下不再命中。
func TestRunScanBoxNoMatchOnFlatSeries(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: makeFlatKlines(300)}

	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "4h",
		BarsLimit: 300,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	if got := reps[StrategyBox]; got == nil || got.Matched != 0 {
		t.Fatalf("expected no box match on a zero-amplitude series, got %+v", got)
	}
}

// 振幅门槛注入生效：同一份 8% 振幅的箱体序列在门槛调到 10% 后不再命中。
func TestRunScanBoxAmplitudeInjectionFiltersHits(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: makeBoxKlines(300)}

	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:        "4h",
		BarsLimit:       300,
		BoxAmplitudePct: 10,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	if got := reps[StrategyBox]; got == nil || got.Matched != 0 {
		t.Fatalf("expected no match when amplitude threshold exceeds the box swing, got %+v", got)
	}
}

// 跨度门槛注入生效：同一份 K 线在门槛超过窗口可容纳范围时不再命中。
func TestRunScanBoxMinGapInjectionFiltersHits(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: makeBoxKlines(30)}

	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "1h",
		BarsLimit: 300,
		BoxMinGap: 40, // 超过 30 根 K 线可容纳的跨度
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	if got := reps[StrategyBox]; got == nil || got.Matched != 0 {
		t.Fatalf("expected no match when gap threshold exceeds available bars, got %+v", got)
	}
}

// 单调上行序列相邻根间距 1%，超过带宽阈值，凑不出箱体。
func TestRunScanBoxNoMatchOnTrendingSeries(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: makeRampKlines(300)}

	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "1h",
		BarsLimit: 300,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	if got := reps[StrategyBox]; got == nil || got.Matched != 0 {
		t.Fatalf("expected no box match on a trending series, got %+v", got)
	}
}

func TestRunScanSkipsBoxWhenBarsInsufficient(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "X"}}, klines: makeFlatKlines(3)}

	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:  "1h",
		BarsLimit: 3,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	if reps[StrategyBox] != nil {
		t.Fatalf("expected box skipped when bars insufficient, got %+v", reps[StrategyBox])
	}
}

func makeFlatKlines(n int) []model.Kline {
	out := make([]model.Kline, n)
	start := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	for i := range out {
		out[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * 15 * time.Minute),
			Open:   1,
			Close:  1,
			High:   1,
			Low:    1,
			Volume: 1000,
		}
	}
	return out
}

// makeBoxKlines 构造顶底都成立的箱体序列：低点恒为 1、高点恒为 1.08，跨度内振幅 8%。
func makeBoxKlines(n int) []model.Kline {
	out := make([]model.Kline, n)
	start := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	for i := range out {
		out[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * 15 * time.Minute),
			Open:   1.02,
			Close:  1.05,
			High:   1.08,
			Low:    1,
			Volume: 1000,
		}
	}
	return out
}

// sparseBottomBox 仅底部箱体成立且触及恰好 3 次（用于排序：应排在密集箱体之后）。
func sparseBottomBox(n int) []model.Kline {
	out := make([]model.Kline, n)
	start := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	price := 200.0
	for i := range out {
		out[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * time.Hour),
			Open:   price,
			Close:  price,
			High:   price * 1.001,
			Low:    price,
			Volume: 1000,
		}
		price *= 1.01
	}
	setSparseLow := func(idx int, low float64) {
		out[idx].Low = low
		out[idx].Open = low * 1.002
		out[idx].Close = low * 1.003
		out[idx].High = low * 1.005
	}
	// 与 box.TestEvalBottomHit 同结构：3 次触及、跨度足够；基准上行序列本身提供 ≥5% 振幅。
	setSparseLow(n-20, 100.0)
	setSparseLow(n-12, 100.4)
	setSparseLow(n-2, 100.2) // 末根已收盘
	return out
}

func makeRampKlines(n int) []model.Kline {
	out := make([]model.Kline, n)
	start := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	price := 1.0
	for i := range out {
		out[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * time.Hour),
			Open:   price,
			Close:  price,
			High:   price,
			Low:    price,
			Volume: 1000,
		}
		price *= 1.01
	}
	return out
}

func TestRunTrendReport(t *testing.T) {
	bars := makeTrendBullBars(300)
	src := multiIntervalSource{
		assets: []Asset{{Symbol: "BTCUSDT", Name: "BTC"}},
		klines: map[string][]model.Kline{
			"BTCUSDT|15m": bars,
			"BTCUSDT|1h":  bars,
			"BTCUSDT|4h":  bars,
		},
	}
	rep, err := NewService(src).RunTrend(context.Background(), ScanJob{
		BarsLimit: 300,
		Workers:   2,
		Assets:    src.assets,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep == nil || rep.Pattern != "trend" || rep.Mode != "1h:trend" || rep.Period != "1h" {
		t.Fatalf("unexpected report meta: %+v", rep)
	}
	if rep.Title != "多周期趋势" || rep.Matched != 1 {
		t.Fatalf("unexpected title/matched: %+v", rep)
	}
	if rep.Results[0].Code != "BTCUSDT" || !strings.Contains(rep.Results[0].Tag, "多头") {
		t.Fatalf("unexpected result: %+v", rep.Results[0])
	}
}

func TestSortResultsTrendByGapDesc(t *testing.T) {
	results := []model.Result{
		{Code: "AAA", Alert: true, Snapshot: model.Snapshot{EMA30: 110, EMA60: 100, EMA120: 95}},  // sum ≈ 14
		{Code: "BBB", Alert: false, Snapshot: model.Snapshot{EMA30: 120, EMA60: 100, EMA120: 80}}, // sum ≈ 37
		{Code: "CCC", Alert: false, Snapshot: model.Snapshot{EMA30: 110, EMA60: 100, EMA120: 95}}, // sum ≈ 14
	}
	sortResults(StrategyTrend, results)
	if results[0].Code != "BBB" {
		t.Fatalf("expected largest gap first, got %v", codesOf(results))
	}
	if results[1].Code != "AAA" || !results[1].Alert {
		t.Fatalf("expected Alert before non-Alert when gap equal, got %v", codesOf(results))
	}
	if results[2].Code != "CCC" {
		t.Fatalf("unexpected order: %v", codesOf(results))
	}
}

func codesOf(results []model.Result) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Code
	}
	return out
}

func TestRunTrendSkipsWhenBarsInsufficient(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "X"}}, klines: makeFlatKlines(300)}
	rep, err := NewService(src).RunTrend(context.Background(), ScanJob{
		BarsLimit: 100,
		Assets:    []Asset{{Symbol: "X"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep != nil {
		t.Fatalf("expected nil report when bars insufficient, got %+v", rep)
	}
}

func TestRunTrendSkipsAssetMissingInterval(t *testing.T) {
	bars := makeTrendBullBars(300)
	src := multiIntervalSource{
		assets: []Asset{{Symbol: "BTCUSDT"}, {Symbol: "ETHUSDT"}},
		klines: map[string][]model.Kline{
			"BTCUSDT|15m": bars,
			"BTCUSDT|1h":  bars,
			"BTCUSDT|4h":  bars,
			// ETH 缺 4h → 跳过
			"ETHUSDT|15m": bars,
			"ETHUSDT|1h":  bars,
		},
	}
	rep, err := NewService(src).RunTrend(context.Background(), ScanJob{
		BarsLimit: 300,
		Workers:   2,
		Assets:    src.assets,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep == nil || rep.Matched != 1 || rep.Results[0].Code != "BTCUSDT" {
		t.Fatalf("expected only BTCUSDT, got %+v", rep)
	}
}

// makeTrendBullBars 构造可通过 trend.Eval 多头判定的指数上涨序列，并强制 1h 影线。
func makeTrendBullBars(n int) []model.Kline {
	out := make([]model.Kline, n)
	start := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	price := 100.0
	for i := range out {
		out[i] = model.Kline{
			Date:   start.Add(time.Duration(i) * time.Hour),
			Open:   price,
			Close:  price,
			High:   price,
			Low:    price,
			Volume: 1000,
		}
		price *= 1.01
	}
	closes := model.Closes(out)
	e30 := indicator.EMA(closes, 30)
	sig := n - 2
	c := out[sig].Close
	out[sig].Open = c
	out[sig].Close = c
	out[sig].High = c
	out[sig].Low = e30[sig] - 3
	return out
}
