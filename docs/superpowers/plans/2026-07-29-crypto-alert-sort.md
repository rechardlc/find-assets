# Crypto Alert-First Sort Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 所有数字货币策略报告结果统一按「强势 → Touches → Code/Tag」排序，邮件与导出共用该顺序。

**Architecture:** 排序发生在 `internal/crypto/service.go` 的 `RunScan` 组装 `exporter.Report` 之前；邮件 / console / md / json 只消费 `Results` 顺序，不在 `notify` 二次排序。

**Tech Stack:** Go 1.x、`sort.Slice`、`go test ./internal/crypto/...`

**Spec:** `docs/superpowers/specs/2026-07-29-crypto-alert-sort-design.md`

## Global Constraints

- 排序键顺序固定：`Alert` 优先 → `Snapshot.Touches` 降序 → `Code` 升序 → `Tag` 升序
- 不做按振幅%/穿线数的策略专属二级键
- 不改 `notify` 包
- 提交仅在用户明确要求时执行（本仓库约定）

## File Structure

| File | Role |
|------|------|
| `internal/crypto/service.go` | 修改 `RunScan` 内 `sort.Slice` 比较器 |
| `internal/crypto/service_test.go` | 新增 Alert 置顶测试；保留现有 Touches 测试 |

---

### Task 1: Alert-first 排序

**Files:**
- Modify: `internal/crypto/service.go`（`RunScan` 内约 L121–130 的 `sort.Slice`）
- Modify: `internal/crypto/service_test.go`（在 `TestRunScanBoxSortsByTouchesDescending` 附近新增测试）

**Interfaces:**
- Consumes: `[]model.Result`（含 `Alert`、`Snapshot.Touches`、`Code`、`Tag`）
- Produces: 同切片原地排序后写入 `Report.Results`

- [ ] **Step 1: Write the failing test**

在 `service_test.go` 增加振幅双合约测试：字母序靠前的合约仅普通命中，靠后的为强势；断言强势排在前面（当前实现按 Code 会把普通命中排前，故应失败）。

```go
// 强势（Alert）优先于代码序：字母序靠前的普通命中不得排在强势之前。
func TestRunScanSortsAlertBeforeNonAlert(t *testing.T) {
	weak := makeFlatKlines(10)
	sig := len(weak) - 2
	weak[sig] = model.Kline{Date: weak[sig].Date, Open: 1, High: 1.1, Low: 1, Close: 1.09, Volume: 2000} // 10%

	strong := makeFlatKlines(10)
	strong[sig] = model.Kline{Date: strong[sig].Date, Open: 1, High: 1.18, Low: 1, Close: 1.17, Volume: 2000} // 18%

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/crypto/ -run TestRunScanSortsAlertBeforeNonAlert -v`

Expected: FAIL（`AAAUSDT` 因代码序排在 `ZZZUSDT` 前，或 `Alert then non-Alert` 断言失败）

- [ ] **Step 3: Write minimal implementation**

将 `service.go` 中比较器改为：

```go
sort.Slice(results, func(i, j int) bool {
	if results[i].Alert != results[j].Alert {
		return results[i].Alert
	}
	if results[i].Snapshot.Touches != results[j].Snapshot.Touches {
		return results[i].Snapshot.Touches > results[j].Snapshot.Touches
	}
	if results[i].Code != results[j].Code {
		return results[i].Code < results[j].Code
	}
	return results[i].Tag < results[j].Tag
})
```

并更新上方注释为：强势优先，其次触及次数降序，再代码/Tag。

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/crypto/ -run "TestRunScanSortsAlertBeforeNonAlert|TestRunScanBoxSortsByTouchesDescending" -v`

Expected: 两个 PASS

再跑：`go test ./internal/crypto/...`

Expected: PASS

- [ ] **Step 5: Commit（仅当用户要求时）**

若用户要求提交：

```bash
git add internal/crypto/service.go internal/crypto/service_test.go docs/superpowers/specs/2026-07-29-crypto-alert-sort-design.md docs/superpowers/plans/2026-07-29-crypto-alert-sort.md
git commit -m "$(cat <<'EOF'
Sort crypto scan results with Alert hits first.

EOF
)"
```

---

## Spec coverage self-review

| Spec 要求 | Task |
|-----------|------|
| Alert 优先 | Task 1 Step 3 |
| Touches 降序 | Task 1 Step 3 + 既有 `TestRunScanBoxSortsByTouchesDescending` |
| Code/Tag 兜底 | Task 1 Step 3 |
| 不改 notify | 无 notify 改动 |
| 测试覆盖 | Task 1 Step 1/4 |
