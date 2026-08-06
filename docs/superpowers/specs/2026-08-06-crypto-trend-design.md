# 数字货币多周期趋势策略（trend）

**日期**：2026-08-06  
**状态**：已实现

## 背景

现有 crypto 策略（reversal / pierce / amplitude / box）均为**单周期**流水线：某一周期收盘后拉一次 K 线，多策略共享判定。新需求需要 **15m + 1h + 4h 联合过滤**，且影线确认只在 1h，不适配单周期 `RunScan`。

范围：**仅数字货币**，不做 A 股。

## 目标

新增固定策略 `trend`：

1. 15m、4h：EMA5/10/30/60/120 中至少 3 根呈方向排列，且子序列**必须包含 EMA60 与 EMA120**
2. 1h：EMA30 / EMA60 / EMA120 严格方向排列
3. 1h：`gap(EMA120,EMA60) > 8%` 且 `gap(EMA60,EMA30) > 8%`
4. 1h 判定根（len-2）影线与 EMA30 交叉：只允许影线，实体不穿（可贴边）
5. 空头为上述全部对称
6. 每根 **1h 收盘后**扫描一次

## 非目标

- 不接入 A 股 / 不改 `internal/strategy`
- 不改造现有单周期 `RunScan` 语义
- 不引入新数据源或数据库
- 不做成交量过滤

## 架构（方案 1）

| 层 | 职责 |
|----|------|
| `internal/crypto/trend` | 纯判定：`Eval(bars15m, bars1h, bars4h)` |
| `Service.RunTrend` | 每合约并发拉 15m/1h/4h → Eval → 独立报告 |
| `cmd/crypto-scanner` | `-trend` 开关；调度挂在 **1h 边界** |

策略标识：`trend`。报告：`Period=1h`，`Pattern=trend`，`Mode=1h:trend`，`Title=多周期趋势`。

判定根：三周期一律 **len-2**（与项目其它策略一致；len-1 为形成中 K 线）。

## 判定规则

均线池（快→慢）：`[EMA5, EMA10, EMA30, EMA60, EMA120]`。  
多头 = 严格递减子序列（快线在上）；空头 = 严格递增。

### 多头

1. **15m 与 4h**（两侧都要）：存在长度 ≥ 3 的严格递减子序列，且包含 EMA60、EMA120。  
   实际含义：`EMA60 > EMA120`，且 `{EMA5,EMA10,EMA30}` 中至少再纳入 1 根，整体严格递减。
2. **1h 排列**：`EMA30 > EMA60 > EMA120`（严格）。
3. **1h 间距**：`gap > 8%`，其中 `gap(a,b) = |a-b| / max(a,b) * 100`（与 pierce 一致）。
4. **1h 下影确认**：`Low ≤ EMA30 ≤ min(Open, Close)`。

### 空头

对称：

1. 15m/4h：长度 ≥ 3 的严格递增子序列，含 EMA60、EMA120。
2. 1h：`EMA30 < EMA60 < EMA120`。
3. 间距阈值相同。
4. 上影：`max(Open, Close) ≤ EMA30 ≤ High`。

同一合约同一次扫描：多头与空头互斥，至多命中一侧。

## CLI / 调度

| 参数 | 默认 | 说明 |
|------|------|------|
| `-trend` | `true` | 是否启用多周期趋势；`-trend=false` 关闭 |

- 定时模式：1h 周期到期时，在现有 1h 单周期扫描之外，额外执行 `RunTrend`
- **调度表**：`-trend=true` 时，即使 `-intervals` / `-pierce-intervals` 等均不含 1h，仍须把 `1h` 纳入调度并在到期时只跑 `RunTrend`
- `-scan-on-start` 且 trend 开启时启动先跑一轮
- 导出与邮件：独立报告；`Matched == 0` 不发邮件（沿用现有）

## 数据与错误

- 每合约拉三周期 K 线，默认 `bars=300`
- 任一侧不足以计算 EMA120（建议阈值与 pierce 对齐，约 ≥250 根，或 `MinRequiredBars`）则跳过该合约
- 单合约拉取失败：打日志并跳过；整池失败才中断
- 不改变 OKX 全局限速语义

## 测试

- `trend`：多头 hit / 空头 hit / 缺 60·120 子序列 miss / 间距不足 miss / 实体穿线 miss / 影线贴边 hit
- `service`：`RunTrend` 产出 `1h:trend` 报告
- CLI：`-trend` 解析与默认值

## 文档

README、`doc/数字货币合约扫描器设计.md` 补充 `trend` 策略与 `-trend` 参数。
