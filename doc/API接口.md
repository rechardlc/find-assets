# API 接口文档

HTTP 服务通过 A 股扫描器启动，基础路径为 `/api/v1`：

```powershell
.\find-assets.exe -s -a=:8080
# 等价：-serve -addr=:8080
```

> 当前 HTTP API **仅面向 A 股**。数字货币使用独立 CLI `crypto-scanner.exe`（无 HTTP），默认拐点 `15m,1h,4h` + 一箭穿心 `15m,1h,4h`，详见 [数字货币合约扫描器设计](数字货币合约扫描器设计.md)。

## 通用说明

- Content-Type：`application/json`
- 跨域：默认允许 `*`（开发联调用）
- 扫描限流：同一时刻仅允许 **1 个**扫描任务运行
- 市场范围：A 股（沪深主板、创业板、科创板）
- 数据源：由启动时 `-source` 决定（默认 `auto` = 东财→新浪→腾讯）；HTTP 请求体不指定数据源

---

## 1. 健康检查

```
GET /api/v1/health
```

**响应**：

```json
{ "status": "ok" }
```

---

## 2. 策略列表

```
GET /api/v1/strategies
```

策略由「周期 × 形态」正交组合。接口枚举 `strategy` 包注册表中的全部周期与形态。

> **注意**：注册表含 `15m`（恒等透传，供扩展），故本接口可能返回 6 组组合。但创建扫描时 `period` 的 binding 仅允许 `day` / `week`；传入 `15m` 会 `400`。A 股数据源只提供日线，请勿对 A 股使用 `15m`。

**响应示例**（实用组合；实际还可能含 `15m:*`）：

```json
{
  "periods": [
    { "name": "15m", "label": "15分钟" },
    { "name": "day", "label": "日线" },
    { "name": "week", "label": "周线" }
  ],
  "patterns": [
    { "name": "pierce", "label": "一箭穿心" },
    { "name": "reversal", "label": "超跌拐点" }
  ],
  "strategies": [
    { "period": "day",  "pattern": "pierce",   "mode": "day:pierce",   "title": "日线一箭穿心" },
    { "period": "day",  "pattern": "reversal", "mode": "day:reversal", "title": "日线超跌拐点" },
    { "period": "week", "pattern": "pierce",   "mode": "week:pierce",  "title": "周线一箭穿心" },
    { "period": "week", "pattern": "reversal", "mode": "week:reversal","title": "周线超跌拐点" }
  ]
}
```

---

## 3. 同步扫描

```
POST /api/v1/scans
```

阻塞执行，完成后直接返回完整报告（约 10~15 秒）。

**请求体**：

```json
{
  "period": "day",
  "pattern": "pierce",
  "workers": 100,
  "range": 2,
  "volume": 3,
  "dead_cross": 3,
  "bars_limit": 600
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| period | string | 是 | 周期：`day` 或 `week` |
| pattern | string | 是 | 形态：`pierce` 或 `reversal` |
| workers | int | 否 | 并发数，默认 100 |
| range | float | 否 | `pierce` 粘合度阈值（百分比，2 = 2%），默认 2 |
| volume | float | 否 | `pierce` 放量阈值（百分比，3 = 较前一根成交量增加 >3%），默认 3 |
| dead_cross | int | 否 | `reversal` 死叉后第几根 K 线触发，默认 3 |
| bars_limit | int | 否 | 拉取日线根数，默认 600 |

**响应**（200）：

```json
{
  "task_id": "8f3a...",
  "period": "day",
  "pattern": "pierce",
  "mode": "day:pierce",
  "title": "日线一箭穿心",
  "started_at": "2026-05-28T14:30:22+08:00",
  "finished_at": "2026-05-28T14:30:31+08:00",
  "elapsed": "9.42s",
  "total": 5320,
  "matched": 3,
  "results": [
    {
      "code": "600519",
      "name": "贵州茅台",
      "tag": "一箭穿心",
      "metric": "粘合度: 0.82%, 放量: 20.00%",
      "snapshot": {
        "date": "2026-05-28",
        "close": 1680.5,
        "ema5": 1679.1,
        "range": 0.82,
        "volume": 120000,
        "prev_volume": 100000,
        "volume_increase": 20
      }
    }
  ]
}
```

**错误**：

- `400`：参数校验失败
- `429`：已有扫描任务在执行
- `500`：扫描失败

---

## 4. 异步扫描

```
POST /api/v1/scans/async
```

立即返回 `task_id`，后台执行扫描。

**请求体**：同同步扫描。

**响应**（202）：

```json
{
  "task_id": "8f3a...",
  "status": "running"
}
```

---

## 5. 查询任务结果

```
GET /api/v1/scans/:id
```

**响应**（200）：

```json
{
  "task_id": "8f3a...",
  "period": "day",
  "pattern": "pierce",
  "status": "done",
  "done": 5320,
  "total": 5320,
  "started_at": "2026-05-28T14:30:22+08:00",
  "finished_at": "2026-05-28T14:30:31+08:00",
  "error": "",
  "report": { ... }
}
```

**status 取值**：`pending` | `running` | `done` | `failed`

---

## 6. SSE 进度流

```
GET /api/v1/scans/:id/stream
```

Content-Type：`text/event-stream`

**事件类型**：

| event | 说明 |
|-------|------|
| snapshot | 首帧，当前任务快照 |
| progress | 进度更新 `{ "done": 1000, "total": 5320, "stage": "scanning" }` |
| end | 任务结束 `{ "status": "done", "error": "" }` |

**示例**：

```
event: progress
data: {"done":2000,"total":5320,"stage":"scanning"}

event: end
data: {"status":"done","error":""}
```

---

## 7. 导出结果

```
GET /api/v1/scans/:id/export?format=json
GET /api/v1/scans/:id/export?format=md
```

| format | Content-Type |
|--------|--------------|
| json | application/json |
| md | text/markdown |

响应头含 `Content-Disposition: attachment`，可直接下载文件。

**错误**：

- `404`：任务不存在或尚未完成
- `400`：未知 format

---

## 8. 调用示例

### cURL

```bash
# 同步扫描：日线一箭穿心（默认粘合度 2%，放量 >3%）
curl -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{"period":"day","pattern":"pierce"}'

# 自定义粘合度为 1.2%
curl -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{"period":"day","pattern":"pierce","range":1.2}'

# 新组合：周线一箭穿心
curl -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{"period":"week","pattern":"pierce"}'

# 异步扫描 + 轮询：周线超跌拐点（可调 dead_cross）
TASK=$(curl -s -X POST http://localhost:8080/api/v1/scans/async \
  -H "Content-Type: application/json" \
  -d '{"period":"week","pattern":"reversal","dead_cross":3}' | jq -r .task_id)

curl http://localhost:8080/api/v1/scans/$TASK

# 下载 Markdown
curl "http://localhost:8080/api/v1/scans/$TASK/export?format=md" -o result.md
```

### PowerShell

```powershell
$body = '{"period":"day","pattern":"pierce"}'
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/scans" `
  -ContentType "application/json" -Body $body
```
