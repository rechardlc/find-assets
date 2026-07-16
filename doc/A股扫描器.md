# A 股扫描器（cmd/scanner）

本文说明 **A 股入口**的调用链、能力边界，以及与「定时日线 / 周五周线」部署目标的对照。运维步骤见 [运维部署](运维部署.md) §12；HTTP 字段见 [API 接口](API接口.md)；策略细则见 [项目规划](项目规划.md)、[技术方案](技术方案.md)。

---

## 1. 入口与模式

| 模式 | 触发 | 行为 |
|------|------|------|
| CLI | 默认（指定 `-period` × `-pattern`） | 单次扫描 → 导出 → 退出 |
| HTTP | `-serve` / `-s` | Gin 常驻；请求体内指定 period/pattern |

一次进程只跑 **一组** `period × pattern`。要跑日线突破 + 日线超跌，需顺序启动两次（或脚本封装）。

二进制产物约定：本机 Windows 多为 `find-assets.exe`；Linux（GCP）建议名为 `scanner`（与 `crypto-scanner` 并列）。

---

## 2. 调用链

### 2.1 CLI

```text
cmd/scanner/main.go
  ├─ expandShortFlags(-p/-pt/-w/-so/…)
  ├─ PrepareStockCache()                    # 可执行文件旁 stocks/ 按日缓存
  ├─ source.NewComposite(spec)              # auto|em|sina|tencent|file:
  │     └─ 有今日缓存时：file:今日.json,原 spec
  └─ runCLI → service.ScanService.Run
        ├─ strategy.Get(period, pattern, Options)
        ├─ src.ListAll                      # 清单（缓存命中则读文件）
        ├─ scanner.Run                      # worker pool
        │     └─ 每只股票：
        │           DailyKlines(n)          # 前复权日线
        │           → Strategy.Match
        │                 Period.Resample   # day 恒等 / week→aggregator.ToWeekly
        │                 Pattern.Eval      # pierce | reversal → indicator.EMA
        ├─ （pierce）按 Snapshot.Range 升序
        ├─ 组装 exporter.Report
        ├─ 首次远程清单 → SaveStocks
        └─ exporter（console / json / md）
```

### 2.2 HTTP

```text
main → runServer → server.NewRouter(ScanService)
  POST /api/v1/scans          # 同步
  POST /api/v1/scans/async    # 异步 + TaskStore
  GET  /api/v1/scans/:id
  GET  /api/v1/scans/:id/stream   # SSE 进度
  GET  /api/v1/scans/:id/export   # 按格式导出
```

HTTP **不走** CLI 的 `PrepareStockCache`；清单每次经 Composite 远程拉取（或请求指定 source）。同时只允许 **1** 个扫描任务。

### 2.3 分层对照（前端类比）

| 层 | 包 | 类比 |
|----|-----|------|
| 入口 | `cmd/scanner` | 页面入口 / CLI 脚本 |
| 编排 | `internal/service` | 业务 hooks / composable |
| 策略 | `internal/strategy` | 纯函数规则引擎 |
| 并发 IO | `internal/scanner` | 有限并发的 data fetching |
| 数据源 | `internal/source` | repository + 多源 fallback |
| 导出 | `internal/exporter` | 序列化 / 下载 |
| HTTP | `internal/server` | router + controller（宜薄） |

---

## 3. 策略组合（已实现）

| period | pattern | 含义 | CLI |
|--------|---------|------|-----|
| day | pierce | 日线一箭穿心（突破） | `-p day -pt pierce` |
| day | reversal | 日线超跌拐点 | `-p day -pt reversal` |
| week | pierce | 周线一箭穿心 | `-p week -pt pierce` |
| week | reversal | 周线超跌拐点 | `-p week -pt reversal` |

注册表另有 `15m`（Resample 恒等），但 **A 股 Source 只提供日线**；HTTP binding 仅 `day`/`week`。

数据要求：前复权日线 OHLCV；周线本地合成。东财 / 新浪 / 腾讯字段已够用（见技术方案 §3.1）。

---

## 4. 能力对照：定时部署目标

目标日程（北京时间，工作日）：

| 时刻 | 任务 |
|------|------|
| 21:05 | 日线 pierce + 日线 reversal（顺序两轮） |
| 周五 21:20 | 周线 pierce 与/或 reversal |

| 能力 | 现状 | 是否满足 |
|------|------|----------|
| 日线突破 / 超跌 | CLI 两套 `-p day -pt …`；`scripts/ashare/scan-day.sh` | ✅ |
| 周线仅周五 | `scripts/ashare/scan-week.sh` + cron 周五 | ✅ |
| 单次扫完退出 | CLI oneshot | ✅ |
| 多策略批跑 | 仓库 shell 顺序调用（进程内仍单组合） | ✅ |
| 命中邮件 | CLI 已接 `notify`（与 crypto 同 SMTP） | ✅ |
| GCP 常驻 HTTP | 支持但不推荐与 crypto 同机 | ⚠️ 用 cron |
| 与 crypto 进程隔离 | 独立二进制与目录 | ✅ |

**结论**：策略、邮件与 cron 脚本已齐；固定按日程跑（不维护节假日清单）。部署见 [运维部署](运维部署.md) §12。

---

## 5. 与 crypto-scanner 的边界

| | A 股 `scanner` | 数字货币 `crypto-scanner` |
|--|----------------|---------------------------|
| 策略包 | `internal/strategy` | `internal/crypto/{pierce,reversal}` |
| 数据源 | 东财/新浪/腾讯 | OKX |
| 调度 | 外部 cron + `scripts/ashare/` | 内置多周期边界 + delay |
| 邮件 | CLI `notify`（`-mail`） | `notify.SendReport` |
| 同机共存 | oneshot | systemd 常驻 |

共享：`model` / `indicator` / `exporter` / `notify`。

---

## 6. 相关文档

| 文档 | 内容 |
|------|------|
| [运维部署](运维部署.md) §12 | GCP 上 A 股 cron 与同机共存 |
| [技术方案](技术方案.md) | 模块与数据流细节 |
| [API 接口](API接口.md) | HTTP REST |
| [项目规划](项目规划.md) | 需求与里程碑 |
| [运维部署](运维部署.md) §1–11 | crypto-scanner（已验证路径） |
