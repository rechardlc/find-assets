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
	fmt.Fprintln(w, "| # | 代码 | 名称 |")
	fmt.Fprintln(w, "|---|---|---|")
	for i, it := range r.Results {
		fmt.Fprintf(w, "| %d | `%s` | %s |\n", i+1, it.Code, it.Name)
	}
	return nil
}
