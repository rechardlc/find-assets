# API 接口文档

HTTP 服务通过 `-serve -addr=:8080` 启动，基础路径为 `/api/v1`。

## 通用说明

- Content-Type：`application/json`
- 跨域：默认允许 `*`（开发联调用）
- 扫描限流：同一时刻仅允许 **1 个**扫描任务运行

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

**响应**：

```json
{
  "strategies": [
    { "mode": "day", "title": "日线一箭穿心" },
    { "mode": "week", "title": "周线超跌拐点" }
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
  "mode": "day",
  "workers": 100,
  "range": 1.5,
  "bars_limit": 600
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| mode | string | 是 | `day` 或 `week` |
| workers | int | 否 | 并发数，默认 100 |
| range | float | 否 | 日线粘合度阈值（百分比，1.5 = 1.5%），默认 1.5；优先于 `cohesion` |
| cohesion | float | 否 | 日线粘合度阈值（小数，0.015 = 1.5%），向后兼容字段，仅在未传 `range` 时生效 |
| bars_limit | int | 否 | 拉取日线根数，默认 600 |

**响应**（200）：

```json
{
  "task_id": "8f3a...",
  "mode": "day",
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
      "tag": "[日线穿心突破]",
      "metric": "粘合度: 0.82%",
      "snapshot": {
        "date": "2026-05-28",
        "close": 1680.5,
        "ema5": 1679.1,
        "cohesion": 0.0082
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
  "mode": "day",
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
# 同步扫描（默认粘合度 1.5%）
curl -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{"mode":"day"}'

# 自定义粘合度为 1.2%
curl -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{"mode":"day","range":1.2}'

# 异步扫描 + 轮询
TASK=$(curl -s -X POST http://localhost:8080/api/v1/scans/async \
  -H "Content-Type: application/json" \
  -d '{"mode":"week"}' | jq -r .task_id)

curl http://localhost:8080/api/v1/scans/$TASK

# 下载 Markdown
curl "http://localhost:8080/api/v1/scans/$TASK/export?format=md" -o result.md
```

### PowerShell

```powershell
$body = '{"mode":"day"}'
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/scans" `
  -ContentType "application/json" -Body $body
```
