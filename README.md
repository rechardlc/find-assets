# 全市场多维度量化选股器

一款面向个人投资者与量化交易员的**本地化、高并发、多策略**资产扫描工具。基于 Go 语言开发，A 股支持命令行（CLI）与 HTTP API（Gin）双模式运行，数字货币合约提供独立 CLI 入口，零运行时依赖，单文件即可部署。

## 功能概览

策略由两个**正交维度**自由组合：**周期（period）** 决定指标算在哪种 K 线上，**形态（pattern）** 决定怎样才算命中。A 股与数字货币市场独立运行，公共复用 EMA、策略与导出能力。

| 维度 | 取值 | 说明 |
|------|------|------|
| 周期 `-period` / `-interval` | `day` / `week` / `15m` | A 股日线 / 周线；数字货币 15 分钟 K 线 |
| 形态 `-pattern` | `pierce` / `reversal` | 一箭穿心 / 超跌拐点 |

A 股常用组合：

| 组合 | 含义 |
|------|------|
| `day` × `pierce` | 日线一箭穿心：均线高度粘合后，放量阳线穿透五条 EMA |
| `week` × `reversal` | 周线超跌拐点：长期空头排列后，关键均线对近期交织 |
| `week` × `pierce` | 周线一箭穿心（新组合） |
| `day` × `reversal` | 日线超跌拐点（新组合） |

- 覆盖沪深主板、创业板、科创板（约 5000+ 只）
- 前复权日线数据，本地合成周线
- 并发扫描（默认 100 协程），单次全量约 15 秒内
- 结果导出：控制台 / JSON / Markdown
- 数字货币：Binance 优先、OKX 回退，扫描 USDT 永续热门山寨合约池
- 数字货币：每日首次生成本地合约池缓存，15m K 线收盘后延迟扫描

## 快速开始

### 打包

```powershell
# A 股扫描器
go build -ldflags="-s -w" -o find-assets.exe ./cmd/scanner

# 数字货币合约扫描器
go build -ldflags="-s -w" -o crypto-scanner.exe ./cmd/crypto-scanner
```

### A 股 CLI 模式

```powershell
# 日线一箭穿心（默认粘合度 2%，放量 20%）
.\find-assets.exe -period=day -pattern=pierce

# 日线一箭穿心，自定义粘合度阈值为 1.2%
.\find-assets.exe -period=day -pattern=pierce -range=1.2

# 周线超跌拐点，并导出 JSON + Markdown
.\find-assets.exe -period=week -pattern=reversal -export=json,md -out=./output

# 新组合：周线一箭穿心
.\find-assets.exe -period=week -pattern=pierce
```

### A 股 HTTP 服务模式

```powershell
.\find-assets.exe -serve -addr=:8080
```

```bash
curl -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{"period":"day","pattern":"pierce"}'
```

### 数字货币合约 CLI 模式

首次使用邮件通知前，复制 `env.example` 为 `.env`，填入 QQ 邮箱 SMTP 授权码：

```powershell
Copy-Item env.example .env
notepad .env
```

`.env` 内容示例：

```powershell
FIND_ASSETS_SMTP_PASS=你的QQ邮箱授权码
```

也可以临时在 PowerShell 设置：

```powershell
$env:FIND_ASSETS_SMTP_PASS="你的QQ邮箱授权码"
```

```powershell
# 默认定时扫描：命中时推送到 rechard.liu@qq.com
.\crypto-scanner.exe

# 单次扫描
.\crypto-scanner.exe -schedule=false

# 导出 JSON + Markdown
.\crypto-scanner.exe -export=json,md -out=./output
```

## A 股命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-period` | day | K 线周期：`day` / `week`（CLI 必填） |
| `-pattern` | pierce | 选股形态：`pierce` / `reversal`（CLI 必填） |
| `-workers` | 100 | 最大并发数 |
| `-bars` | 600 | 拉取日线根数 |
| `-range` | 2 | `pierce` 形态粘合度阈值（百分比，2 = 2%） |
| `-volume` | 20 | `pierce` 形态放量阈值（百分比，20 = 较前一根成交量增加 20%） |
| `-export` | console | 导出格式：`console,json,md` |
| `-out` | ./output | 文件导出目录 |
| `-serve` | false | 启动 HTTP 服务 |
| `-addr` | :8080 | HTTP 监听地址 |

## 数字货币命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-source` | binance,okx | 数据源回退顺序 |
| `-pool` | hot_alt | 合约池规则，当前支持热门山寨综合评分 |
| `-top` | 20 | 每日缓存的候选合约数量 |
| `-interval` | 15m | K 线周期 |
| `-pattern` | reversal | 策略形态 |
| `-bars` | 300 | 每个合约拉取的 K 线数量 |
| `-workers` | 10 | 最大并发数 |
| `-schedule` | true | 是否按周期持续扫描；单次扫描可传 `-schedule=false` |
| `-delay` | 20s | K 线收盘后的延迟执行时间 |
| `-export` | console | 导出格式：`console,json,md` |
| `-out` | ./output | 文件导出目录 |
| `-mail` | true | 命中时发送邮件通知 |
| `-mail-to` | rechard.liu@qq.com | 邮件收件人 |
| `-mail-from` | rechard.liu@qq.com | 邮件发件人 |
| `-smtp-host` | smtp.qq.com | SMTP 服务器 |
| `-smtp-port` | 465 | SMTP 端口 |
| `-smtp-user` | rechard.liu@qq.com | SMTP 用户名 |
| `-smtp-pass` | 环境变量 `FIND_ASSETS_SMTP_PASS` | SMTP 授权码 |
| `-env` | .env | 环境变量文件路径 |

## 项目结构

```
find-assets/
├── cmd/scanner/          # A 股入口（CLI + HTTP 双模式）
├── cmd/crypto-scanner/   # 数字货币 USDT 永续合约入口
├── internal/
│   ├── source/           # 数据源（东方财富）
│   ├── crypto/           # Binance/OKX 合约池、缓存、调度、扫描编排
│   ├── aggregator/       # 日→周 K 线合成
│   ├── indicator/        # EMA 等指标
│   ├── strategy/         # 选股策略：周期(period) × 形态(pattern)
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
| [数字货币合约扫描器设计](doc/数字货币合约扫描器设计.md) | Binance/OKX、hot_alt 合约池、缓存与 15m 调度 |

## 技术栈

- **语言**：Go 1.25+
- **HTTP 框架**：Gin
- **数据源**：东方财富 push2 接口（前复权）、Binance Futures、OKX Public API
- **依赖**：仅 `gin`、`uuid`，其余为标准库

## License

See [LICENSE](LICENSE).
