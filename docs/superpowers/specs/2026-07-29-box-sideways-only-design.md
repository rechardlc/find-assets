# 箱体仅保留窄幅横盘（顶底同时命中）

**日期**：2026-07-29  
**状态**：已实现

## 问题

箱体策略对顶部、底部独立判定；两侧同时命中时已合并为一条「窄幅横盘」。仅底或仅顶仍会进入报告并发邮件，噪音偏多。

## 目标

新增布尔开关（默认 `true`）：为 true 时，报告层（console / md / json / 邮件）**只保留顶底同时命中的合并结果**；为 false 时保持现有行为（仅底、仅顶、窄幅横盘均可出）。

## 设计

### 参数

| 层 | 名称 | 默认 |
|----|------|------|
| CLI | `-box-sideways-only` | `true` |
| `ScanJob` | `BoxSidewaysOnly` | 由 CLI 注入；未接 CLI 的调用方若零值则为 `false`（Go bool 零值）。**crypto-scanner 默认显式传 `true`** |

> 说明：`ScanJob` 用 `bool` 零值为 false，与「CLI 默认 true」不冲突——入口 `parseConfig` / `ScanJob` 构造处写死默认 true。

### 过滤位置（方案 A）

在 `evalBox` 内：

- `hasBottom && hasTop` → 仍 `MergeSideways`，始终返回
- 仅底或仅顶 → 若 `BoxSidewaysOnly` 为 true 则返回 `nil`；否则返回单侧结果

不在 `notify` 二次过滤。

### 零命中

过滤后 `Matched == 0` 时沿用现有逻辑：不发邮件。

### 测试

- 默认 / `BoxSidewaysOnly=true`：仅底序列（如 `sparseBottomBox`）不进结果；顶底双命中仍进
- `BoxSidewaysOnly=false`：仅底仍可进
- 更新依赖「仅底 + 窄幅横盘」双命中排序的测试：改为 `false`，或换成两个窄幅横盘样本

### 文档

README、`doc/数字货币合约扫描器设计.md` 补一行 `-box-sideways-only`。

## 范围外

- 不改 `box.Eval` / `MergeSideways` 判定本身
- 不做按 Tag 字符串后过滤
