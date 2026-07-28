package crypto

import (
	"context"
	"testing"
	"time"

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
	// 全平序列：窗口内每根 K 线都踩在同一价位，顶部与底部箱体各命中一次。
	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: makeFlatKlines(300)}

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
	if rep.Title != "4小时箱体震荡(带宽≤0.6%·触及≥3次)" {
		t.Fatalf("unexpected box title: %q", rep.Title)
	}
	if rep.Matched != 2 {
		t.Fatalf("expected bottom + top box on a flat series, got %d", rep.Matched)
	}
}

func TestRunScanBoxUsesInjectedOptions(t *testing.T) {
	src := fakeSource{assets: []Asset{{Symbol: "PEPEUSDT"}}, klines: makeFlatKlines(300)}

	reps, err := NewService(src).RunScan(context.Background(), ScanJob{
		Interval:   "1h",
		BarsLimit:  300,
		BoxPct:     1.2,
		BoxTouches: 4,
	}, []string{StrategyBox})
	if err != nil {
		t.Fatal(err)
	}
	if got := reps[StrategyBox]; got == nil || got.Title != "1小时箱体震荡(带宽≤1.2%·触及≥4次)" {
		t.Fatalf("injected box options not reflected in report: %+v", got)
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
