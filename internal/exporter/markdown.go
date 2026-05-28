package exporter

import (
	"fmt"
	"io"
)

type Markdown struct{}

func (Markdown) Format() string      { return "md" }
func (Markdown) ContentType() string { return "text/markdown; charset=utf-8" }

func (Markdown) Write(w io.Writer, r *Report) error {
	fmt.Fprintf(w, "# 选股结果 · %s 策略\n\n", r.Title)
	fmt.Fprintf(w, "- 任务 ID：`%s`\n", r.TaskID)
	fmt.Fprintf(w, "- 扫描时间：%s\n", r.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "- 扫描股票数：**%d**\n", r.Total)
	fmt.Fprintf(w, "- 命中数：**%d**\n", r.Matched)
	fmt.Fprintf(w, "- 耗时：%s\n\n", r.Elapsed)

	if len(r.Results) == 0 {
		fmt.Fprintln(w, "> 当前未发现符合条件的标的。")
		return nil
	}

	fmt.Fprintln(w, "## 命中清单")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| # | 代码 | 名称 | 标签 | 指标 | 日期 | 收盘 | EMA5 | EMA10 | EMA30 | EMA60 | EMA120 |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|---|---|")
	for i, it := range r.Results {
		s := it.Snapshot
		ema120 := "-"
		if s.EMA120 > 0 {
			ema120 = fmt.Sprintf("%.2f", s.EMA120)
		}
		fmt.Fprintf(w,
			"| %d | `%s` | %s | %s | %s | %s | %.2f | %.2f | %.2f | %.2f | %.2f | %s |\n",
			i+1, it.Code, it.Name, it.Tag, it.Metric,
			s.Date, s.Close, s.EMA5, s.EMA10, s.EMA30, s.EMA60, ema120,
		)
	}
	return nil
}
