# Crypto Scanner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an independent USDT perpetual crypto scanner that reuses the existing strategy and exporter layers while preserving the current A-share scanner behavior.

**Architecture:** Add `cmd/crypto-scanner` plus an `internal/crypto` package. Keep exchange-specific API clients behind a small source interface, cache the daily candidate pool locally, and feed normalized 15m K lines into the existing strategy package.

**Tech Stack:** Go standard library, existing `internal/strategy`, existing `internal/exporter`, Binance Futures public API, OKX public API.

---

## File Structure

- Create `doc/数字货币合约扫描器设计.md`: user-facing design document.
- Create `internal/crypto/model.go`: normalized crypto asset, ticker metric, pool cache, and scan parameter types.
- Create `internal/crypto/pool.go`: `hot_alt` filtering, scoring, and sorting.
- Create `internal/crypto/pool_test.go`: TDD tests for score sorting and exclusion filters.
- Create `internal/crypto/cache.go`: daily pool cache path, stale cleanup, JSON load/save.
- Create `internal/crypto/cache_test.go`: TDD tests for cache path and same-day reuse.
- Create `internal/crypto/scheduler.go`: next aligned run time for 15m delayed scans.
- Create `internal/crypto/scheduler_test.go`: TDD tests for boundary alignment.
- Create `internal/crypto/source.go`: crypto source interface and fallback composite.
- Create `internal/crypto/service.go`: scan orchestration from cached assets to report.
- Create `cmd/crypto-scanner/main.go`: independent CLI entrypoint.
- Modify `internal/strategy/period.go`: add `15m` period as identity resample.
- Modify `internal/strategy/strategy.go`: register `15m`.

## Task 1: Hot Alt Pool Logic

**Files:**
- Create: `internal/crypto/model.go`
- Create: `internal/crypto/pool.go`
- Test: `internal/crypto/pool_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestBuildHotAltPoolFiltersMajorsAndSortsByScore(t *testing.T) {
	metrics := []Metric{
		{Symbol: "BTCUSDT", ExchangeSymbol: "BTCUSDT", Base: "BTC", Quote: "USDT", Status: "TRADING", PriceChangePercent: 20, High24h: 120, Low24h: 80, Open24h: 100, QuoteVolume: 900000000, FundingRate: 0.001},
		{Symbol: "DOGEUSDT", ExchangeSymbol: "DOGEUSDT", Base: "DOGE", Quote: "USDT", Status: "TRADING", PriceChangePercent: -12, High24h: 1.3, Low24h: 0.9, Open24h: 1, QuoteVolume: 200000000, FundingRate: -0.0005},
		{Symbol: "PEPEUSDT", ExchangeSymbol: "PEPEUSDT", Base: "PEPE", Quote: "USDT", Status: "TRADING", PriceChangePercent: -18, High24h: 0.000013, Low24h: 0.000008, Open24h: 0.00001, QuoteVolume: 500000000, FundingRate: -0.0012},
		{Symbol: "HALTUSDT", ExchangeSymbol: "HALTUSDT", Base: "HALT", Quote: "USDT", Status: "BREAK", PriceChangePercent: -30, High24h: 2, Low24h: 1, Open24h: 1.5, QuoteVolume: 700000000, FundingRate: -0.002},
	}
	got := BuildHotAltPool(metrics, PoolOptions{Top: 2, ExcludeMajors: true})
	if len(got) != 2 {
		t.Fatalf("expected 2 assets, got %d: %+v", len(got), got)
	}
	if got[0].Symbol != "PEPEUSDT" {
		t.Fatalf("expected PEPE first, got %+v", got[0])
	}
	if got[1].Symbol != "DOGEUSDT" {
		t.Fatalf("expected DOGE second, got %+v", got[1])
	}
}
```

- [ ] **Step 2: Run red test**

Run: `go test ./internal/crypto -run TestBuildHotAltPoolFiltersMajorsAndSortsByScore -count=1`

Expected: FAIL because `internal/crypto` and `BuildHotAltPool` do not exist.

- [ ] **Step 3: Implement minimal pool types and logic**

Define `Asset`, `Metric`, `PoolOptions`, `PoolCache`, `BuildHotAltPool`, `Amplitude`, and major/stable filters.

- [ ] **Step 4: Run green test**

Run: `go test ./internal/crypto -run TestBuildHotAltPoolFiltersMajorsAndSortsByScore -count=1`

Expected: PASS.

## Task 2: Daily Pool Cache

**Files:**
- Create: `internal/crypto/cache.go`
- Test: `internal/crypto/cache_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestPreparePoolCacheAtReusesTodayAndRemovesOldPoolCaches(t *testing.T) {
	dir := t.TempDir()
	oldDir := PoolCacheDirAt(dir)
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(oldDir, "hot_alt_20000101_binance.json")
	if err := os.WriteFile(oldPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	todayPath := TodayPoolCachePathAt(dir, "hot_alt", "binance")
	if err := os.WriteFile(todayPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, use, err := PreparePoolCacheAt(dir, "hot_alt", "binance")
	if err != nil {
		t.Fatal(err)
	}
	if !use || got != todayPath {
		t.Fatalf("expected reuse today cache %q, got %q use=%v", todayPath, got, use)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old cache removed, stat err=%v", err)
	}
}
```

- [ ] **Step 2: Run red test**

Run: `go test ./internal/crypto -run TestPreparePoolCacheAtReusesTodayAndRemovesOldPoolCaches -count=1`

Expected: FAIL because cache helpers do not exist.

- [ ] **Step 3: Implement cache helpers**

Implement cache directory `crypto/pools`, file name `pool_YYYYMMDD_exchange.json`, stale cleanup, `SavePoolCache`, and `LoadPoolCache`.

- [ ] **Step 4: Run green test**

Run: `go test ./internal/crypto -run TestPreparePoolCacheAtReusesTodayAndRemovesOldPoolCaches -count=1`

Expected: PASS.

## Task 3: 15m Delayed Scheduler

**Files:**
- Create: `internal/crypto/scheduler.go`
- Test: `internal/crypto/scheduler_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestNextDelayedRunAlignsToNextIntervalBoundary(t *testing.T) {
	now := time.Date(2026, 6, 17, 9, 7, 3, 0, time.Local)
	got := NextDelayedRun(now, 15*time.Minute, 20*time.Second)
	want := time.Date(2026, 6, 17, 9, 15, 20, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestNextDelayedRunMovesPastCurrentDelayedBoundary(t *testing.T) {
	now := time.Date(2026, 6, 17, 9, 15, 21, 0, time.Local)
	got := NextDelayedRun(now, 15*time.Minute, 20*time.Second)
	want := time.Date(2026, 6, 17, 9, 30, 20, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("want %s, got %s", want, got)
	}
}
```

- [ ] **Step 2: Run red test**

Run: `go test ./internal/crypto -run TestNextDelayedRun -count=1`

Expected: FAIL because `NextDelayedRun` does not exist.

- [ ] **Step 3: Implement scheduler math**

Implement a pure `NextDelayedRun(now time.Time, interval, delay time.Duration) time.Time` function.

- [ ] **Step 4: Run green test**

Run: `go test ./internal/crypto -run TestNextDelayedRun -count=1`

Expected: PASS.

## Task 4: 15m Strategy Registration

**Files:**
- Modify: `internal/strategy/period.go`
- Modify: `internal/strategy/strategy.go`
- Test: `internal/strategy/strategy_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestGet_15mReversalUsesIdentityPeriod(t *testing.T) {
	s := mustGet(t, "15m", "reversal", Options{})
	if s.Period() != "15m" {
		t.Fatalf("expected 15m period, got %q", s.Period())
	}
	if s.Title() != "15分钟超跌拐点" {
		t.Fatalf("unexpected title: %q", s.Title())
	}
}
```

- [ ] **Step 2: Run red test**

Run: `go test ./internal/strategy -run TestGet_15mReversalUsesIdentityPeriod -count=1`

Expected: FAIL because `15m` period is unknown.

- [ ] **Step 3: Implement 15m period**

Add `Minute15Period` with identity resample and register it in `Periods()` and `resolvePeriod`.

- [ ] **Step 4: Run green test**

Run: `go test ./internal/strategy -run TestGet_15mReversalUsesIdentityPeriod -count=1`

Expected: PASS.

## Task 5: Crypto Service and CLI Skeleton

**Files:**
- Create: `internal/crypto/source.go`
- Create: `internal/crypto/service.go`
- Create: `cmd/crypto-scanner/main.go`
- Test: `internal/crypto/service_test.go`

- [ ] **Step 1: Write failing service test**

Create a fake source returning two assets and deterministic K lines, then assert the service returns an exporter report with period `15m`, pattern `reversal`, and total equal to asset count.

- [ ] **Step 2: Run red test**

Run: `go test ./internal/crypto -run TestServiceRunBuildsReport -count=1`

Expected: FAIL because `Service` does not exist.

- [ ] **Step 3: Implement service and source interface**

Implement `Source`, `CompositeSource`, `Service`, and `Params` with no real network clients yet.

- [ ] **Step 4: Run green test**

Run: `go test ./internal/crypto -run TestServiceRunBuildsReport -count=1`

Expected: PASS.

- [ ] **Step 5: Add CLI skeleton**

Parse flags, prepare cache, run one scan or scheduled scans. Until real clients are wired, return a clear error if no live source is available.

## Task 6: Verification

**Files:**
- All changed Go files
- New docs

- [ ] **Step 1: Format**

Run: `gofmt -w internal/crypto/*.go internal/strategy/*.go cmd/crypto-scanner/*.go`

Expected: no output.

- [ ] **Step 2: Test**

Run: `go test ./...`

Expected: all packages pass.

- [ ] **Step 3: Lints**

Use IDE diagnostics for changed files and fix introduced issues.

