# 全市场多维度量化选股器

一款面向个人投资者与量化交易员的**本地化、高并发、多策略**资产扫描工具。基于 Go 语言开发：

- **A 股**：`cmd/scanner` — CLI + HTTP API（Gin）双模式
- **数字货币 USDT 永续**：`cmd/crypto-scanner` — 独立 CLI（多周期调度 + 邮件通知）

零运行时依赖，单文件即可部署。两套入口共享 `indicator` / `exporter` / `model`，策略与数据源完全隔离。

## 功能概览

### A 股：周期 × 形态

| 维度 | 取值 | 说明 |
|------|------|------|
| 周期 `-period` (`-p`) | `day` / `week` | 日线 / 周线（周线由日线本地合成） |
| 形态 `-pattern` (`-pt`) | `pierce` / `reversal` | 一箭穿心 / 超跌拐点 |

| 组合 | 含义 |
|------|------|
| `day` × `pierce` | 日线一箭穿心（默认） |
| `week` × `reversal` | 周线超跌拐点 |
| `week` × `pierce` | 周线一箭穿心 |
| `day` × `reversal` | 日线超跌拐点 |

- 覆盖沪深主板、创业板、科创板（约 5000+ 只）
- 数据源可回退：`auto` = 东财 → 新浪 → 腾讯；支持 `file:` 本地清单
- 股票清单按日落盘到可执行文件旁 `stocks/`，同日复用
- 并发扫描（默认 100 协程），单次全量约 15 秒内
- 结果导出：控制台 / JSON / Markdown
- 命中可发邮件（`-mail`，同 crypto 的 SMTP / `.env`）
- 定时批跑：`scripts/ashare/` + cron（固定日程，不考虑节假日）

> 策略注册表里还有 `15m` 周期（恒等透传），但 A 股数据源只提供日线；HTTP API 仅接受 `day`/`week`。

### 数字货币：周期 × 策略（独立实现）

| 策略 | 默认周期 | 模块 |
|------|----------|------|
| 拐点 `reversal`（超跌 + 超涨） | `15m,1h,4h` | `internal/crypto/reversal` |
| 一箭穿心 `pierce`（上穿 + 下穿） | `1h,4h` | `internal/crypto/pierce` |
| 振幅异动 `amplitude`（情绪涨 + 情绪跌） | `4h` | `internal/crypto/amplitude` |
| 箱体震荡 `box`（底部 + 顶部箱体） | `4h` | `internal/crypto/box` |
| 多周期趋势 `trend`（多头 + 空头） | 固定 15m+1h+4h（每 1h 收盘） | `internal/crypto/trend` |

- OKX 单一数据源，`hot_alt` 热门山寨合约池（排除 BTC/ETH/稳定币）
- **每次进程启动**清除当日合约池缓存并重新拉取；同日进程内后续扫描复用
- 多周期边界对齐后延迟扫描（默认 `delay=20s`）；同周期 K 线只拉一次，启用的策略合并判定
- 命中时按「周期 × 策略」独立发邮件（QQ SMTP）

## 链路总览

```text
┌─ cmd/scanner (A 股) ─────────────────────────────────────────────┐
│  CLI / HTTP(-serve)                                              │
│       ↓                                                          │
│  source.Composite (auto|em|sina|tencent|file) + stocks/ 日缓存   │
│       ↓                                                          │
│  service.ScanService.Run                                         │
│       ↓                                                          │
│  strategy.Get(period×pattern) → scanner.Run                      │
│       ↓                          ↓                               │
│  Period.Resample            Pattern.Eval                         │
│  (day|week→aggregator)      (pierce|reversal → indicator.EMA)    │
│       ↓                                                          │
│  exporter (console/json/md)                                      │
│  HTTP: Gin /api/v1/scans[+async] + SSE + export                  │
└──────────────────────────────────────────────────────────────────┘

┌─ cmd/crypto-scanner (USDT 永续) ─────────────────────────────────┐
│  .env → parseConfig → OKX Source (限速+重试)                     │
│       ↓                                                          │
│  ClearTodayPoolCache → hot_alt 池 / 自定义列表                   │
│       ↓                                                          │
│  调度：InitSchedule(四组 intervals 并集 ∪ trend→1h) + delay      │
│       ↓                                                          │
│  crypto.Service.RunScan(interval,                                │
│      [reversal?,pierce?,amplitude?,box?])                        │
│  + RunTrend(15m+1h+4h) when -trend & 1h due                      │
│       ↓            ↓             ↓             ↓                 │
│  crypto/reversal crypto/pierce crypto/amplitude crypto/box       │
│  crypto/trend                                                    │
│       ↓                                                          │
│  exporter + notify.SendReport（命中才发）                         │
└──────────────────────────────────────────────────────────────────┘
```

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
# 日线一箭穿心（默认粘合度 2%，放量 >3%）
.\find-assets.exe -p=day -pt=pierce

# 自定义粘合度；短选项与完整名等价
.\find-assets.exe -period=day -pattern=pierce -range=1.2

# 周线超跌拐点，死叉后第 3 根触发；导出 JSON + Markdown
.\find-assets.exe -p=week -pt=reversal -dc=3 -e=json,md -o=./output

# 指定数据源（默认 auto = em→sina→tencent）
.\find-assets.exe -p=day -pt=pierce -so=em
```

### A 股 HTTP 服务模式

```powershell
.\find-assets.exe -s -a=:8080
# 等价：-serve -addr=:8080
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

```text
FIND_ASSETS_SMTP_PASS=你的QQ邮箱授权码
```

也可以临时在 PowerShell 设置：

```powershell
$env:FIND_ASSETS_SMTP_PASS="你的QQ邮箱授权码"
```

```powershell
# 默认定时扫描：启动先扫一轮，再按 15m/1h/4h 边界持续跑；命中发邮件
.\crypto-scanner.exe

# 单次扫描（并集周期各跑一轮后退出）
.\crypto-scanner.exe -schedule=false

# 导出 JSON + Markdown
.\crypto-scanner.exe -export=json,md -out=./output

# 读取本地自定义交易对列表（默认文件：crypto_symbols.txt）
.\crypto-scanner.exe -custom=true -schedule=false
```

`crypto_symbols.txt` 内容示例：

```text
BTCUSDT
ETHUSDT
SOLUSDT
```

## A 股命令行参数

| 参数（简写） | 默认值 | 说明 |
|--------------|--------|------|
| `-period` (`-p`) | day | K 线周期：`day` / `week` |
| `-pattern` (`-pt`) | pierce | 选股形态：`pierce` / `reversal` |
| `-workers` (`-w`) | 100 | 最大并发数 |
| `-bars` (`-b`) | 600 | 拉取日线根数 |
| `-range` (`-r`) | 2 | `pierce` 粘合度阈值（百分比，2 = 2%） |
| `-volume` (`-v`) | 3 | `pierce` 放量阈值（百分比，3 = 较前一根成交量增加 >3%） |
| `-deadcross` (`-dc`) | 3 | `reversal` 死叉后第几根 K 线触发 |
| `-export` (`-e`) | console | 导出格式：`console,json,md` |
| `-out` (`-o`) | ./output | 文件导出目录 |
| `-serve` (`-s`) | false | 启动 HTTP 服务 |
| `-addr` (`-a`) | :8080 | HTTP 监听地址 |
| `-source` (`-so`) | auto | 数据源：`auto` / `em` / `sina` / `tencent` / `file:./path.json`，可逗号串联回退 |
| `-mail` | true | 命中时发送邮件（未配置 `FIND_ASSETS_SMTP_PASS` 则跳过） |
| `-mail-to` / `-mail-from` | richard_0525@foxmail.com | 收件人 / 发件人 |
| `-smtp-host` / `-smtp-port` / `-smtp-user` / `-smtp-pass` | smtp.qq.com / 465 / … | SMTP；密码优先环境变量 |
| `-env` | .env | 环境变量文件 |
| `-help` (`-h`) | false | 显示帮助 |

CLI 自动维护可执行文件旁 `stocks/stocks_YYYYMMDD.json` 清单缓存。定时批跑脚本：`scripts/ashare/scan-day.sh`、`scan-week.sh`。

## 数字货币命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-source` | okx | 数字货币数据源（当前仅支持 okx） |
| `-pool` | hot_alt | 合约池规则，当前支持热门山寨综合评分 |
| `-top` | 200 | 每日缓存的候选合约数量 |
| `-intervals` | 15m,1h,4h | 拐点策略 K 线周期列表 |
| `-pierce-intervals` | 1h,4h | 一箭穿心策略 K 线周期列表；留空关闭 |
| `-amplitude-intervals` | 4h | 振幅异动策略 K 线周期列表，可多周期；留空关闭 |
| `-amplitude` | 9 | 振幅异动阈值（百分比）：上一根 K 线 (最高-最低)/开盘 |
| `-box-intervals` | 4h | 箱体震荡策略 K 线周期列表，可多周期；留空关闭 |
| `-box-pct` | 0.6 | 箱体带宽上限（百分比）：箱体内最高/最低价的最大相差幅度 |
| `-box-lookback` | 24 | 箱体震荡回看的已收盘 K 线根数 |
| `-box-touches` | 3 | 箱体最少触及次数：几根 K 线踩在同一价位才算箱体 |
| `-box-min-gap` | 6 | 首末两次触及之间的中间 K 线最少根数，滤掉连续几根挤在一起的伪箱体 |
| `-box-amplitude` | 5 | 箱体跨度内振幅下限（百分比）：跨度内 (最高-最低)/最低，滤掉贴着均价的死水横盘 |
| `-box-sideways-only` | false | 为 true 时仅输出顶底同时命中的窄幅横盘；默认保留仅底/仅顶 |
| `-trend` | true | 多周期趋势（15m+1h+4h 联合；每 1h 收盘扫描）；`false` 关闭 |
| `-trend-gap` | 1 | 多周期趋势 1h EMA 间距阈值（%）；影线须与 EMA30 交叉或相等；间距>8% 标为强势 |
| `-bars` | 300 | 每个合约拉取的 K 线数量 |
| `-workers` | 10 | 最大并发数 |
| `-schedule` | true | 是否按周期持续扫描；单次扫描传 `-schedule=false` |
| `-delay` | 20s | K 线收盘后的延迟执行时间 |
| `-export` | console | 导出格式：`console,json,md` |
| `-out` | ./output | 文件导出目录 |
| `-mail` | true | 命中时发送邮件通知 |
| `-mail-to` | richard_0525@foxmail.com | 邮件收件人 |
| `-mail-from` | richard_0525@foxmail.com | 邮件发件人 |
| `-smtp-host` | smtp.qq.com | SMTP 服务器 |
| `-smtp-port` | 465 | SMTP 端口 |
| `-smtp-user` | richard_0525@foxmail.com | SMTP 用户名 |
| `-smtp-pass` | 环境变量 `FIND_ASSETS_SMTP_PASS` | SMTP 授权码 |
| `-env` | .env | 环境变量文件路径 |
| `-custom` | false | 是否读取本地自定义数字货币列表 |
| `-custom-file` | ./crypto_symbols.txt | 本地自定义列表，一行一个交易对 |
| `-scan-on-start` | true | 定时模式下启动即先扫描一轮 |
| `-rate` | 15 | OKX 请求全局限速（次/秒），`<=0` 不限速 |

## 项目结构

```
find-assets/
├── cmd/
│   ├── scanner/              # A 股入口（CLI + HTTP）
│   │   ├── main.go
│   │   └── flags.go          # 短选项展开（-p/-pt/-so/-dc …）
│   └── crypto-scanner/       # 数字货币 USDT 永续入口
│       ├── main.go           # 调度、导出、邮件
│       ├── env.go            # .env 加载
│       └── custom.go         # 自定义交易对列表
├── internal/
│   ├── model/                # Stock / Kline / Result
│   ├── source/               # A 股：Composite + 东财/新浪/腾讯/文件 + 清单缓存
│   ├── aggregator/           # 日 → 周 K 线合成
│   ├── indicator/            # EMA、金叉/死叉判定（两市场共用）
│   ├── strategy/             # A 股：period × pattern（pierce / reversal）
│   ├── scanner/              # A 股并发扫描器
│   ├── service/              # A 股 ScanService 编排
│   ├── server/               # Gin HTTP（仅 A 股）
│   ├── exporter/             # console / json / md
│   ├── notify/               # SMTP 邮件（crypto-scanner + A 股 CLI）
│   └── crypto/               # 数字货币编排
│       ├── reversal/         # 专用拐点（超跌/超涨）
│       ├── pierce/           # 专用一箭穿心（上穿/下穿）
│       ├── amplitude/        # 专用振幅异动（情绪涨/情绪跌）
│       ├── box/              # 专用箱体震荡（底部/顶部箱体）
│       ├── trend/            # 多周期趋势（15m+1h+4h）
│       ├── pool.go           # hot_alt 评分
│       ├── cache.go          # 合约池日缓存
│       ├── scheduler.go      # 多周期延迟调度
│       ├── exchange.go       # OKX REST
│       └── service.go        # RunScan 合并编排
├── scripts/ashare/           # 日线/周线 cron 脚本
├── doc/                      # 项目文档（含 A股扫描器.md、运维部署.md）
└── output/                   # 导出结果（运行时生成）
```

## 文档

| 文档 | 说明 |
|------|------|
| [项目规划](doc/项目规划.md) | 背景、目标、功能需求、里程碑 |
| [技术方案](doc/技术方案.md) | A 股架构、模块与数据流（crypto 见专文） |
| [A股扫描器](doc/A股扫描器.md) | `cmd/scanner` 调用链、能力对照、与 crypto 边界 |
| [API 接口](doc/API接口.md) | A 股 HTTP REST API |
| [运维部署](doc/运维部署.md) | GCP：crypto 常驻 + A 股 cron（§12） |
| [数字货币合约扫描器设计](doc/数字货币合约扫描器设计.md) | OKX、hot_alt、四种策略、调度与邮件 |

### GCP 同机定时（摘要）

在已有 `crypto-scanner` 的 e2-micro 上，A 股用 **cron + `scripts/ashare/`**（勿 `-serve`）：

| 北京时间 | 任务 |
|----------|------|
| 工作日 21:05 | 日线 pierce → 日线 reversal |
| 周五 21:20 | 周线 pierce / reversal |

脚本自带 `-mail`；详见 [运维部署 §12](doc/运维部署.md)、[A股扫描器](doc/A股扫描器.md)。

## 技术栈

- **语言**：Go 1.25+
- **HTTP 框架**：Gin（仅 A 股）
- **数据源**：东财 / 新浪 / 腾讯（A 股，可回退）；OKX Public API（数字货币）
- **依赖**：`gin`、`uuid`、`golang.org/x/time`（OKX 限速），其余为标准库

## License

See [LICENSE](LICENSE).
