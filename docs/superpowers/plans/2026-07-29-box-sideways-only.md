# Box Sideways-Only Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 箱体策略默认只输出顶底同时命中的「窄幅横盘」；`-box-sideways-only=false` 恢复仅底/仅顶。

**Architecture:** CLI 布尔注入 `ScanJob.BoxSidewaysOnly`；`evalBox` 在仅单侧命中且开关为 true 时返回空。报告 / 邮件共用过滤后的 `Results`。

**Tech Stack:** Go flag、`go test ./internal/crypto/... ./cmd/crypto-scanner/...`

**Spec:** `docs/superpowers/specs/2026-07-29-box-sideways-only-design.md`

## Global Constraints

- CLI 默认 `true`；`ScanJob` 零值为 `false`（库调用未设字段时保持旧行为）
- crypto-scanner 构造 `ScanJob` 时显式传入 `cfg.boxSidewaysOnly`
- 不改 `box.Eval` / `MergeSideways`
- 提交仅在用户明确要求时执行

## File Structure

| File | Role |
|------|------|
| `internal/crypto/service.go` | `ScanJob.BoxSidewaysOnly` + `evalBox` 过滤 |
| `internal/crypto/service_test.go` | 过滤开/关测试 |
| `cmd/crypto-scanner/main.go` | flag + 注入 |
| `cmd/crypto-scanner/main_test.go` | 默认 true 断言 |
| `README.md` / `doc/数字货币合约扫描器设计.md` | 参数说明一行 |

---

### Task 1: Service 过滤

**Files:**
- Modify: `internal/crypto/service.go`
- Modify: `internal/crypto/service_test.go`

**Interfaces:**
- Consumes: `ScanJob.BoxSidewaysOnly bool`
- Produces: `evalBox(..., sidewaysOnly bool) []model.Result` — 仅单侧且 `sidewaysOnly` 时返回 nil

- [ ] **Step 1: Write the failing test**

```go
func TestRunScanBoxSidewaysOnlyFiltersSingleSide(t *testing.T) {
	ms := multiKlineSource{
		assets: []Asset{{Symbol: "LOWUSDT"}, {Symbol: "BOTHUSDT"}},
		klines: map[string][]model.Kline{
			"LOWUSDT":  sparseBottomBox(30),
			"BOTHUSDT": makeBoxKlines(300),
		},
	}
	reps, err := NewService(ms).RunScan(context.Background(), ScanJob{
		Interval:         "4h",
		BarsLimit:        300,
		BoxSidewaysOnly:  true,
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/crypto/ -run TestRunScanBoxSidewaysOnlyFiltersSingleSide -v`

Expected: FAIL（`BoxSidewaysOnly` 未接线或未知字段 / Matched 仍为 2）

- [ ] **Step 3: Minimal implementation**

1. `ScanJob` 增加 `BoxSidewaysOnly bool` 及注释。
2. `buildActiveStrategies` 箱体分支：

```go
sidewaysOnly := job.BoxSidewaysOnly
evalAsset: func(st model.Stock, ks []model.Kline) []model.Result {
    return evalBox(st, ks, opt, sidewaysOnly)
},
```

3. `evalBox`：

```go
func evalBox(stock model.Stock, klines []model.Kline, opt box.Options, sidewaysOnly bool) []model.Result {
	bottom, hasBottom := box.Eval(stock, klines, box.Bottom, opt)
	top, hasTop := box.Eval(stock, klines, box.Top, opt)
	switch {
	case hasBottom && hasTop:
		return []model.Result{box.MergeSideways(bottom, top, opt.Interval)}
	case hasBottom:
		if sidewaysOnly {
			return nil
		}
		return []model.Result{bottom}
	case hasTop:
		if sidewaysOnly {
			return nil
		}
		return []model.Result{top}
	default:
		return nil
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/crypto/ -run "TestRunScanBox" -v`  
再跑：`go test ./internal/crypto/...`  
Expected: PASS（排序测试依赖零值 `false`，仅底仍保留）

---

### Task 2: CLI + 文档

**Files:**
- Modify: `cmd/crypto-scanner/main.go`
- Modify: `cmd/crypto-scanner/main_test.go`
- Modify: `README.md`（`-box-*` 参数表加一行）
- Modify: `doc/数字货币合约扫描器设计.md`（参数表加一行）

- [ ] **Step 1: Failing CLI default test**

在 `TestParseConfigBoxDefaults` 增加：

```go
if !cfg.boxSidewaysOnly {
	t.Fatal("expected -box-sideways-only default true")
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./cmd/crypto-scanner/ -run TestParseConfigBoxDefaults -v`  
Expected: FAIL（无字段）

- [ ] **Step 3: Wire flag**

`config` 加 `boxSidewaysOnly bool`；

```go
fs.BoolVar(&cfg.boxSidewaysOnly, "box-sideways-only", true, "箱体仅输出顶底同时命中的窄幅横盘；false 时保留仅底/仅顶")
```

`ScanJob` 构造加 `BoxSidewaysOnly: cfg.boxSidewaysOnly`。

文档两处各加一行：`box-sideways-only` / `true` / 说明同上。

- [ ] **Step 4: Verify**

Run: `go test ./cmd/crypto-scanner/ ./internal/crypto/...`  
Expected: PASS

- [ ] **Step 5: Commit（仅当用户要求时）**

---

## Spec coverage

| Spec | Task |
|------|------|
| `-box-sideways-only` 默认 true | Task 2 |
| `evalBox` 过滤 | Task 1 |
| 报告层统一 | Task 1（Results 源） |
| false 恢复旧行为 | Task 1 测试 |
| 文档 | Task 2 |
