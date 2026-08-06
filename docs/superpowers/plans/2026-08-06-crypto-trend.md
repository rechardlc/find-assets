# Crypto Multi-Timeframe Trend Strategy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在数字货币扫描器中新增多周期联合策略 `trend`：15m+4h 锚定排列、1h 三线排列与间距、1h 影线确认 EMA30，每根 1h 收盘后扫描。

**Architecture:** 独立包 `internal/crypto/trend` 做纯判定；`Service.RunTrend` 每合约并发拉 15m/1h/4h；CLI `-trend` 默认开启，调度强制纳入 1h。不改动现有单周期 `RunScan`。

**Tech Stack:** Go、`internal/indicator.EMA`、现有 `exporter`/`notify`、`go test`

**Spec:** `docs/superpowers/specs/2026-08-06-crypto-trend-design.md`

## Global Constraints

- 仅数字货币；不改 `internal/strategy` / A 股
- 判定根三周期一律 `len-2`；`len-1` 为形成中 K 线
- 均线池：`EMA5, EMA10, EMA30, EMA60, EMA120`
- 间距：`gap = |a-b|/max(a,b)*100`，阈值 `> 8`
- 多头影线：`Low <= EMA30 <= min(Open,Close)`；空头：`max(Open,Close) <= EMA30 <= High`
- CLI `-trend` 默认 `true`；开启时调度表必须含 `1h`
- 提交仅在用户明确要求时执行（本计划 Step「Commit」改为「暂存说明，等用户指示再 commit」）

## File Structure

| File | Role |
|------|------|
| `internal/crypto/trend/trend.go` | 判定：`Eval`、排列、间距、影线 |
| `internal/crypto/trend/trend_test.go` | 单元 / 集成判定测试 |
| `internal/crypto/service.go` | `StrategyTrend`、`RunTrend`、排序 |
| `internal/crypto/service_test.go` | `RunTrend` 报告测试 |
| `cmd/crypto-scanner/main.go` | `-trend`、调度、导出邮件 |
| `cmd/crypto-scanner/main_test.go` | flag 默认 / 关闭 |
| `README.md` | 策略表与参数一行 |
| `doc/数字货币合约扫描器设计.md` | 策略章节与参数表 |

---

### Task 1: `trend` 判定包（TDD）

**Files:**
- Create: `internal/crypto/trend/trend.go`
- Create: `internal/crypto/trend/trend_test.go`

**Interfaces:**
- Consumes: `indicator.EMA`、`model.Stock`、`model.Kline`、`model.Result`
- Produces:
  - `const DefaultMinGapPct = 8`
  - `const DefaultMinBars = 250`
  - `type Direction string`（`Bull` / `Bear`）
  - `type Options struct { MinBars int; MinGapPct float64 }`
  - `func DefaultOptions() Options`
  - `func MinRequiredBars(opt Options) int`
  - `func Eval(stock model.Stock, bars15m, bars1h, bars4h []model.Kline, opt Options) (model.Result, bool)`
  - 内部：`hasAnchorArrangement(vals [5]float64, bull bool) bool`、`gapPct(a,b float64) float64`、`wickTouches(k model.Kline, ema30 float64, bull bool) bool`

- [ ] **Step 1: Write failing helper tests**

```go
package trend

import (
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

func TestGapPct(t *testing.T) {
	// |110-100|/110*100 ≈ 9.09
	if g := gapPct(110, 100); g < 9.0 || g > 9.2 {
		t.Fatalf("gapPct=%.4f", g)
	}
}

func TestHasAnchorArrangementBull(t *testing.T) {
	// EMA5>EMA10>EMA60>EMA120，缺 EMA30 仍长度 4，含 60/120
	vals := [5]float64{120, 115, 90, 110, 100} // 5,10,30,60,120 — 子序列 5,10,60,120
	if !hasAnchorArrangement(vals, true) {
		t.Fatal("expected bull anchor arrangement")
	}
	// 仅 5>60>120 但 10、30 乱序：子序列 5,60,120 即可
	vals2 := [5]float64{120, 50, 40, 110, 100}
	if !hasAnchorArrangement(vals2, true) {
		t.Fatal("expected bull with EMA5+60+120")
	}
	// 缺：60 < 120，无法含两者的递减子序列
	vals3 := [5]float64{130, 125, 120, 100, 110}
	if hasAnchorArrangement(vals3, true) {
		t.Fatal("expected miss when EMA60 < EMA120")
	}
}

func TestWickTouchesBull(t *testing.T) {
	k := model.Kline{Open: 110, Close: 112, High: 115, Low: 100}
	if !wickTouches(k, 105, true) {
		t.Fatal("expected lower wick touch")
	}
	// 实体穿过：min(O,C)=108 < ema30=109，不允许
	k2 := model.Kline{Open: 108, Close: 112, High: 115, Low: 100}
	if wickTouches(k2, 109, true) {
		t.Fatal("expected miss when body crosses EMA30")
	}
	// 贴边：min(O,C)=EMA30
	k3 := model.Kline{Open: 105, Close: 110, High: 112, Low: 100}
	if !wickTouches(k3, 105, true) {
		t.Fatal("expected hit when body edge equals EMA30")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL（未定义符号）**

Run: `go test ./internal/crypto/trend -count=1`

Expected: FAIL（package / symbols missing）

- [ ] **Step 3: Implement helpers + package skeleton**

```go
package trend

import (
	"fmt"

	"github.com/find-assets/scanner/internal/indicator"
	"github.com/find-assets/scanner/internal/model"
)

const (
	DefaultMinGapPct = 8
	DefaultMinBars   = 250
)

type Direction string

const (
	Bull Direction = "bull"
	Bear Direction = "bear"
)

type Options struct {
	MinBars    int
	MinGapPct  float64
}

func DefaultOptions() Options {
	return Options{MinBars: DefaultMinBars, MinGapPct: DefaultMinGapPct}
}

func MinRequiredBars(opt Options) int {
	if opt.MinBars <= 0 {
		return DefaultMinBars
	}
	return opt.MinBars
}

func gapPct(a, b float64) float64 {
	hi := a
	if b > hi {
		hi = b
	}
	if hi == 0 {
		return 0
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff / hi * 100
}

// hasAnchorArrangement：vals 按快→慢 [5,10,30,60,120]。
// 要求存在长度>=3 的严格方向子序列，且必须包含下标 3(EMA60) 与 4(EMA120)。
func hasAnchorArrangement(vals [5]float64, bull bool) bool {
	// 枚举：必须含 index 3 与 4；再从 {0,1,2} 选非空子集，检查整条链严格有序
	for mask := 1; mask < 8; mask++ {
		idxs := make([]int, 0, 5)
		for i := 0; i < 3; i++ {
			if mask&(1<<i) != 0 {
				idxs = append(idxs, i)
			}
		}
		idxs = append(idxs, 3, 4)
		ok := true
		for j := 1; j < len(idxs); j++ {
			prev, cur := vals[idxs[j-1]], vals[idxs[j]]
			if bull {
				if !(prev > cur) {
					ok = false
					break
				}
			} else {
				if !(prev < cur) {
					ok = false
					break
				}
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func wickTouches(k model.Kline, ema30 float64, bull bool) bool {
	bodyLo, bodyHi := k.Open, k.Close
	if bodyLo > bodyHi {
		bodyLo, bodyHi = bodyHi, bodyLo
	}
	if bull {
		return k.Low <= ema30 && ema30 <= bodyLo
	}
	return bodyHi <= ema30 && ema30 <= k.High
}
```

- [ ] **Step 4: Re-run helper tests — expect PASS**

Run: `go test ./internal/crypto/trend -run "TestGapPct|TestHasAnchorArrangementBull|TestWickTouchesBull" -count=1`

Expected: PASS

- [ ] **Step 5: Write failing Eval tests（用可控序列）**

构造思路：生成足够长的单调 ramp 使 1h 上 EMA30/60/120 分离；再改写三组序列的 `len-2` OHLC 以满足/破坏影线。为降低对真实 EMA 的脆弱依赖，可在测试里：

1. 用 `rampBars(n, start, step)` 生成上涨/下跌序列；
2. 对命中用例，在写入后用 `indicator.EMA` 读出 `ema30[len-2]`，再 `setWick(bars, ema30, bull)` 改写判定根 OHLC。

```go
func TestEvalBullHit(t *testing.T) {
	bars15 := rampBars(300, 100, 1)
	bars1h := rampBars(300, 100, 1)
	bars4h := rampBars(300, 100, 1)
	forceWickBull(bars1h) // 按当前 ema30 改写 len-2 下影
	r, ok := Eval(model.Stock{Code: "BTCUSDT", Name: "BTC"}, bars15, bars1h, bars4h, DefaultOptions())
	if !ok {
		t.Fatal("expected bull hit")
	}
	if !strings.Contains(r.Tag, "多头") {
		t.Fatalf("tag=%q", r.Tag)
	}
	if r.Snapshot.EMA30 == 0 || r.Snapshot.EMA60 == 0 || r.Snapshot.EMA120 == 0 {
		t.Fatal("expected EMA snapshot")
	}
}

func TestEvalGapTooSmallMiss(t *testing.T) {
	// 平坦序列 → 间距远小于 8%
	bars := flatBars(300, 100)
	if _, ok := Eval(model.Stock{Code: "X"}, bars, bars, bars, DefaultOptions()); ok {
		t.Fatal("expected miss on flat")
	}
}

func TestEvalBodyCrossMiss(t *testing.T) {
	bars15 := rampBars(300, 100, 1)
	bars1h := rampBars(300, 100, 1)
	bars4h := rampBars(300, 100, 1)
	forceBodyCrossBull(bars1h) // 实体穿过 ema30
	if _, ok := Eval(model.Stock{Code: "X"}, bars15, bars1h, bars4h, DefaultOptions()); ok {
		t.Fatal("expected miss when body crosses")
	}
}

func TestEvalBearHit(t *testing.T) {
	bars15 := rampBars(300, 500, -1)
	bars1h := rampBars(300, 500, -1)
	bars4h := rampBars(300, 500, -1)
	forceWickBear(bars1h)
	r, ok := Eval(model.Stock{Code: "X"}, bars15, bars1h, bars4h, DefaultOptions())
	if !ok || !strings.Contains(r.Tag, "空头") {
		t.Fatalf("expected bear hit, got ok=%v tag=%q", ok, r.Tag)
	}
}
```

辅助函数放在同文件：`rampBars` / `flatBars` / `forceWickBull` / `forceWickBear` / `forceBodyCrossBull`（实现时按 `ema30[n-2]` 设置 OHLC）。

- [ ] **Step 6: Run Eval tests — expect FAIL**

Run: `go test ./internal/crypto/trend -run TestEval -count=1`

Expected: FAIL（`Eval` 未实现或恒 false）

- [ ] **Step 7: Implement `Eval`**

```go
func Eval(stock model.Stock, bars15m, bars1h, bars4h []model.Kline, opt Options) (model.Result, bool) {
	if opt.MinBars <= 0 {
		opt.MinBars = DefaultMinBars
	}
	if opt.MinGapPct <= 0 {
		opt.MinGapPct = DefaultMinGapPct
	}
	minN := opt.MinBars
	if len(bars15m) < minN || len(bars1h) < minN || len(bars4h) < minN {
		return model.Result{}, false
	}

	for _, dir := range []Direction{Bull, Bear} {
		if r, ok := evalDir(stock, bars15m, bars1h, bars4h, dir, opt); ok {
			return r, true
		}
	}
	return model.Result{}, false
}

func evalDir(stock model.Stock, b15, b1h, b4h []model.Kline, dir Direction, opt Options) (model.Result, bool) {
	bull := dir == Bull
	e15 := emasAt(b15, len(b15)-2)
	e4h := emasAt(b4h, len(b4h)-2)
	e1h := emasAt(b1h, len(b1h)-2)
	if e15 == nil || e4h == nil || e1h == nil {
		return model.Result{}, false
	}
	if !hasAnchorArrangement(*e15, bull) || !hasAnchorArrangement(*e4h, bull) {
		return model.Result{}, false
	}
	// 1h: EMA30,60,120 → indices 2,3,4
	if bull {
		if !(e1h[2] > e1h[3] && e1h[3] > e1h[4]) {
			return model.Result{}, false
		}
	} else {
		if !(e1h[2] < e1h[3] && e1h[3] < e1h[4]) {
			return model.Result{}, false
		}
	}
	if gapPct(e1h[4], e1h[3]) <= opt.MinGapPct || gapPct(e1h[3], e1h[2]) <= opt.MinGapPct {
		return model.Result{}, false
	}
	k := b1h[len(b1h)-2]
	if !wickTouches(k, e1h[2], bull) {
		return model.Result{}, false
	}
	label := "多头"
	if !bull {
		label = "空头"
	}
	tag := fmt.Sprintf("[多周期趋势·%s]", label)
	metric := fmt.Sprintf("1h gap60/120=%.2f%% gap30/60=%.2f%%",
		gapPct(e1h[4], e1h[3]), gapPct(e1h[3], e1h[2]))
	return model.Result{
		Code:   stock.Code,
		Name:   stock.Name,
		Tag:    tag,
		Metric: metric,
		Snapshot: model.Snapshot{
			Date:   k.Date.Format("2006-01-02 15:04"),
			Close:  k.Close,
			High:   k.High,
			Low:    k.Low,
			EMA5:   e1h[0],
			EMA10:  e1h[1],
			EMA30:  e1h[2],
			EMA60:  e1h[3],
			EMA120: e1h[4],
			Bars:   len(b1h),
		},
	}, true
}

func emasAt(bars []model.Kline, idx int) *[5]float64 {
	if idx < 0 || idx >= len(bars) {
		return nil
	}
	closes := model.Closes(bars)
	e5 := indicator.EMA(closes, 5)
	e10 := indicator.EMA(closes, 10)
	e30 := indicator.EMA(closes, 30)
	e60 := indicator.EMA(closes, 60)
	e120 := indicator.EMA(closes, 120)
	return &[5]float64{e5[idx], e10[idx], e30[idx], e60[idx], e120[idx]}
}
```

- [ ] **Step 8: Run all trend tests — expect PASS**

Run: `go test ./internal/crypto/trend -count=1`

Expected: PASS。若 ramp 未拉开 8% 间距，增大 `step` 或 `n`，或在断言前打印 `gapPct` 调试，直到稳定通过。

- [ ] **Step 9: 暂存（等用户指示再 commit）**

```bash
git add internal/crypto/trend/
# 勿自动 commit
```

---

### Task 2: `Service.RunTrend`

**Files:**
- Modify: `internal/crypto/service.go`
- Modify: `internal/crypto/service_test.go`

**Interfaces:**
- Consumes: `trend.Eval`、`trend.MinRequiredBars`、`Source.Klines`
- Produces:
  - `const StrategyTrend = "trend"`
  - `func (s *Service) RunTrend(ctx context.Context, job ScanJob) (*exporter.Report, error)`
  - 排序：复用 `lessResult` 默认分支（Alert → Touches → Code → Tag）；trend 无 Touches 时主要按 Code

- [ ] **Step 1: Write failing service test**

在 `service_test.go` 增加按 interval 返回 K 线的 fake：

```go
type multiIntervalSource struct {
	assets []Asset
	// key: symbol+"|"+interval
	klines map[string][]model.Kline
}

func (m multiIntervalSource) Name() string { return "fake" }
func (m multiIntervalSource) ListAssets(context.Context) ([]Asset, error) {
	return m.assets, nil
}
func (m multiIntervalSource) Klines(_ context.Context, asset Asset, interval string, _ int) ([]model.Kline, error) {
	ks, ok := m.klines[asset.Symbol+"|"+interval]
	if !ok {
		return nil, fmt.Errorf("no klines")
	}
	return ks, nil
}

func TestRunTrendReport(t *testing.T) {
	// 使用与 trend_test 相同的命中构造；导出辅助到测试内复制一份，或在 trend 包提供 TestHelpers（不推荐导出）。
	// 最简：三周期同一组 bull ramp + forceWickBull。
	bars := makeTrendBullBars(300) // 测试文件内复制 ramp+forceWick
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
}
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `go test ./internal/crypto -run TestRunTrendReport -count=1`

Expected: FAIL（`RunTrend` undefined）

- [ ] **Step 3: Implement `RunTrend`**

在 `service.go`：

```go
import "github.com/find-assets/scanner/internal/crypto/trend"

const StrategyTrend = "trend"

func (s *Service) RunTrend(ctx context.Context, job ScanJob) (*exporter.Report, error) {
	if s.src == nil {
		return nil, errors.New("数字货币数据源未配置")
	}
	if job.BarsLimit <= 0 {
		job.BarsLimit = 300
	}
	if job.Workers <= 0 {
		job.Workers = 10
	}
	opt := trend.DefaultOptions()
	minBars := trend.MinRequiredBars(opt)
	if job.BarsLimit < minBars {
		return nil, nil // 与长周期不足跳过一致：整次跳过
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

	sem := make(chan struct{}, job.Workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []model.Result

	intervals := []string{"15m", "1h", "4h"}

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

			var bars [3][]model.Kline
			for i, iv := range intervals {
				ks, err := s.src.Klines(ctx, asset, iv, job.BarsLimit)
				if err != nil || len(ks) < minBars {
					return
				}
				bars[i] = ks
			}
			stock := model.Stock{Code: asset.Symbol, Name: asset.Name}
			if r, ok := trend.Eval(stock, bars[0], bars[1], bars[2], opt); ok {
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}
		}(asset)
	}
	wg.Wait()

	sortResults(StrategyTrend, results)
	finishedAt := time.Now()
	return &exporter.Report{
		AssetClass: exporter.AssetCrypto,
		Period:     "1h",
		Pattern:    "trend",
		Mode:       "1h:trend",
		Title:      "多周期趋势",
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Elapsed:    finishedAt.Sub(startedAt).Round(10 * time.Millisecond).String(),
		Total:      len(assets),
		Matched:    len(results),
		Results:    results,
	}, nil
}
```

- [ ] **Step 4: Run service tests — expect PASS**

Run: `go test ./internal/crypto -run TestRunTrend -count=1`

Expected: PASS

- [ ] **Step 5: 暂存（等用户指示再 commit）**

```bash
git add internal/crypto/service.go internal/crypto/service_test.go
```

---

### Task 3: CLI 开关与 1h 调度

**Files:**
- Modify: `cmd/crypto-scanner/main.go`
- Modify: `cmd/crypto-scanner/main_test.go`

**Interfaces:**
- Consumes: `crypto.Service.RunTrend`、`crypto.ParseIntervalList` / `SpecByName`
- Produces: `config.trend bool`；`allSpecs` 在 `trend` 开启时并入 `1h`

- [ ] **Step 1: Write failing CLI tests**

```go
func TestParseConfigTrendDefaultTrue(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.trend {
		t.Fatal("expected -trend default true")
	}
}

func TestParseConfigTrendDisable(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")
	cfg, err := parseConfig([]string{"-trend=false"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.trend {
		t.Fatal("expected trend disabled")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./cmd/crypto-scanner -run TestParseConfigTrend -count=1`

Expected: FAIL（`cfg.trend` undefined）

- [ ] **Step 3: Wire CLI**

1. `config` 增加 `trend bool`
2. `fs.BoolVar(&cfg.trend, "trend", true, "启用多周期趋势策略（15m+1h+4h 联合；每 1h 收盘扫描）")`
3. 构建 `allSpecs` 后：

```go
allSpecs := unionSpecs(cfg.intervals, cfg.pierceIntervals, cfg.amplitudeIntervals, cfg.boxIntervals)
if cfg.trend {
	if _, ok := crypto.SpecByName(allSpecs, "1h"); !ok {
		h, err := crypto.ParseIntervalList("1h")
		if err != nil {
			log.Fatal(err) // 或 parseConfig 返回 error
		}
		allSpecs = unionSpecs(allSpecs, h)
	}
}
```

若 `ParseIntervalList` 不宜在 `main` 里 `Fatal`，把并入逻辑放进 `parseConfig` 并返回 error。

4. 抽取 `runTrend()`：

```go
runTrend := func() {
	if !cfg.trend {
		return
	}
	var assets []crypto.Asset
	var err error
	if cfg.custom {
		assets, err = loadCustomAssets(cfg.customFile)
	} else {
		assets, err = loadOrBuildPool(ctx, src, cfg.pool, cfg.top)
	}
	if err != nil {
		log.Printf("[trend] 合约池准备失败: %v", err)
		return
	}
	rep, err := svc.RunTrend(ctx, crypto.ScanJob{
		BarsLimit: cfg.bars,
		Workers:   cfg.workers,
		Assets:    assets,
	})
	if err != nil {
		log.Printf("[trend] 扫描失败: %v", err)
		return
	}
	if rep == nil {
		log.Printf("[trend] K 线根数不足，跳过")
		return
	}
	if err := dispatchExports(rep, splitFormats(cfg.exportArg), cfg.outDir, "crypto_1h_trend"); err != nil {
		log.Printf("[trend] 导出失败: %v", err)
	}
	if err := maybeSendMail(cfg, rep); err != nil {
		log.Printf("[trend] 邮件通知失败: %v", err)
	}
}
```

5. 调度调用点：

- 单次 / `scan-on-start`：在遍历 `allSpecs` 的 `runInterval` **之外**，若 `cfg.trend` 则调用一次 `runTrend()`（避免每个 interval 都跑；启动一轮只跑一次）。
- 定时循环：当 `due` 含 `"1h"` 时，先/后调用 `runTrend()`（与该小时 `runInterval("1h")` 并列）。

注意：`scan-on-start` 与 `-schedule=false` 全量扫描时 **只调用一次** `runTrend`，不要对每个 due interval 重复。

推荐结构：

```go
runAllOnce := func() {
	for _, spec := range allSpecs {
		runInterval(spec.Name)
	}
	runTrend()
}
// schedule=false / scanOnStart → runAllOnce()
// timer due: for _, name := range due { runInterval(name); if name=="1h" { runTrend() }; Advance... }
```

- [ ] **Step 4: Run CLI tests — expect PASS**

Run: `go test ./cmd/crypto-scanner -run TestParseConfigTrend -count=1`

Expected: PASS

- [ ] **Step 5: 全量相关测试**

Run: `go test ./internal/crypto/... ./cmd/crypto-scanner -count=1`

Expected: PASS

- [ ] **Step 6: 暂存**

```bash
git add cmd/crypto-scanner/main.go cmd/crypto-scanner/main_test.go
```

---

### Task 4: 文档

**Files:**
- Modify: `README.md`
- Modify: `doc/数字货币合约扫描器设计.md`

- [ ] **Step 1: README**

在数字货币策略表增加一行：

| 多周期趋势 `trend` | 固定 15m+1h+4h（每 1h 收盘） | `internal/crypto/trend` |

参数表增加：

| `-trend` | `true` | 多周期趋势；`false` 关闭 |

- [ ] **Step 2: 设计文档**

在 `doc/数字货币合约扫描器设计.md`：

- 目标列表增加 trend 一条
- 参数表增加 `-trend`
- 新增小节「多周期趋势」：复述 spec 判定规则（len-2、锚定排列、间距 8%、影线、调度）

- [ ] **Step 3: 将 spec 状态改为「已实现」**（实现全部通过后）

修改 `docs/superpowers/specs/2026-08-06-crypto-trend-design.md` 状态字段。

- [ ] **Step 4: 暂存；等用户指示后统一 commit**

```bash
git add README.md doc/数字货币合约扫描器设计.md docs/superpowers/specs/2026-08-06-crypto-trend-design.md
```

---

## Spec Coverage Checklist

| Spec 要求 | Task |
|-----------|------|
| 15m/4h 锚定排列含 EMA60+120 | Task 1 |
| 1h EMA30>60>120（及空头对称） | Task 1 |
| 1h 双间距 >8% | Task 1 |
| 1h 影线确认（可贴边、实体不穿） | Task 1 |
| len-2 | Task 1 |
| 仅 crypto | 全部 |
| `RunTrend` 拉三周期 | Task 2 |
| `-trend` 默认 true | Task 3 |
| 调度强制 1h | Task 3 |
| 独立报告/邮件 | Task 3 |
| README + 设计文档 | Task 4 |

## Self-Review Notes

- 无 TBD；`Eval` / `RunTrend` / CLI 签名与任务间一致
- `makeTrendBullBars` 在 Task 2 测试中需自包含（复制 Task 1 辅助函数，避免跨包导出测试工具）
- 若 `rampBars` 无法稳定产生 >8% gap，实现时优先调大 step，而不是降低阈值
