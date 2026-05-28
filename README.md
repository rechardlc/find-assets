# 全市场多维度量化选股器

一款面向个人投资者与量化交易员的**本地化、高并发、多策略**智能选股工具。基于 Go 语言开发，支持命令行（CLI）与 HTTP API（Gin）双模式运行，零运行时依赖，单文件即可部署。

## 功能概览

| 策略 | 模式 | 说明 |
|------|------|------|
| 周线超跌拐点 | `-mode=week` | 寻找大级别趋势拐点，EMA 交织后空头排列 |
| 日线一箭穿心 | `-mode=day` | 均线高度粘合后，阳线穿透五条 EMA |

- 覆盖沪深主板、创业板、科创板（约 5000+ 只）
- 前复权日线数据，本地合成周线
- 并发扫描（默认 100 协程），单次全量约 15 秒内
- 结果导出：控制台 / JSON / Markdown

## 快速开始

### 打包

```powershell
go build -ldflags="-s -w" -o 选股器.exe ./cmd/scanner
```

### CLI 模式

```powershell
# 日线策略
.\选股器.exe -mode=day

# 周线策略，并导出 JSON + Markdown
.\选股器.exe -mode=week -export=json,md -out=./output
```

### HTTP 服务模式

```powershell
.\选股器.exe -serve -addr=:8080
```

```bash
curl -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{"mode":"day"}'
```

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-mode` | — | 策略模式：`day` / `week`（CLI 必填） |
| `-workers` | 100 | 最大并发数 |
| `-bars` | 600 | 拉取日线根数 |
| `-cohesion` | 0.015 | 日线策略粘合度阈值（1.5%） |
| `-export` | console | 导出格式：`console,json,md` |
| `-out` | ./output | 文件导出目录 |
| `-serve` | false | 启动 HTTP 服务 |
| `-addr` | :8080 | HTTP 监听地址 |

## 项目结构

```
find-assets/
├── cmd/scanner/          # 程序入口（CLI + HTTP 双模式）
├── internal/
│   ├── source/           # 数据源（东方财富）
│   ├── aggregator/       # 日→周 K 线合成
│   ├── indicator/        # EMA 等指标
│   ├── strategy/         # 选股策略 day / week
│   ├── scanner/          # 并发扫描器
│   ├── service/          # 业务编排层
│   ├── server/           # Gin HTTP 服务
│   └── exporter/         # 结果导出
├── doc/                  # 项目文档
└── output/               # 导出结果（运行时生成）
```

## 文档

| 文档 | 说明 |
|------|------|
| [项目规划](doc/项目规划.md) | 背景、目标、功能需求、里程碑 |
| [技术方案](doc/技术方案.md) | 架构设计、模块说明、算法与数据流 |
| [API 接口](doc/API接口.md) | HTTP REST API 说明 |
| [打包部署](doc/打包部署.md) | 编译、跨平台打包与运行 |

## 技术栈

- **语言**：Go 1.25+
- **HTTP 框架**：Gin
- **数据源**：东方财富 push2 接口（前复权）
- **依赖**：仅 `gin`、`uuid`，其余为标准库

## License

See [LICENSE](LICENSE).
