package exporter

import (
	"fmt"
	"io"
)

type Console struct{}

func (Console) Format() string      { return "console" }
func (Console) ContentType() string { return "text/plain; charset=utf-8" }

func (Console) Write(w io.Writer, r *Report) error {
	fmt.Fprintln(w, "================ 筛选完成 ================")
	for _, it := range r.Results {
		fmt.Fprintf(w, "%s %s\n", it.Code, it.Name)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "符合条件的股票共计: %d 只。 耗时: %s\n", r.Matched, r.Elapsed)
	return nil
}
