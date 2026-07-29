# Crypto 扫描结果强势优先排序

**日期**：2026-07-29  
**状态**：已实现

## 问题

`cmd/crypto-scanner` 各策略命中后，邮件正文按 `Report.Results` 顺序展示。当前 `RunScan` 仅按箱体 `Touches` 降序（其余策略 Touches=0 时退化为代码序），**未**把 `Alert=true`（★强势）统一置顶。

## 目标

所有数字货币策略（reversal / pierce / amplitude / box）的报告结果统一排序，强势在前；邮件、console、md、json 共用同一顺序。

## 排序规则

在 `internal/crypto/service.go` 的 `RunScan` 组装报告前：

1. `Alert=true` 优先于 `Alert=false`
2. `Snapshot.Touches` 降序（箱体有效；其它策略多为 0）
3. `Code` 升序，再 `Tag` 升序（稳定兜底）

## 范围

- **改**：`internal/crypto/service.go` 排序比较器；`service_test.go` 覆盖 Alert 置顶并保留 Touches 断言
- **不改**：`notify` 层不再二次排序；不做按振幅%/穿线数的策略专属二级键

## 验收

- 混合 Alert 命中时，强势条目排在普通命中之前
- 箱体仍满足触及次数多的在前（同 Alert 级别内）
- `go test ./internal/crypto/...` 通过
