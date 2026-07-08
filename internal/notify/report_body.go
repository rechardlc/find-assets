package notify

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/find-assets/scanner/internal/exporter"
)

var beijingLocation = time.FixedZone("CST", 8*3600)

func writeReportBody(w io.Writer, rep *exporter.Report) error {
	title := strings.TrimSpace(rep.Title)
	if title == "" {
		title = rep.Mode
	}
	fmt.Fprintf(w, "  %s · %s\n", rep.AssetLabel(), title)
	fmt.Fprintln(w)

	if len(rep.Results) == 0 {
		fmt.Fprintln(w, "当前未发现符合条件的标的。")
		return nil
	}

	fmt.Fprintln(w, "命中清单")
	for i, it := range rep.Results {
		fmt.Fprintf(w, "  %2d. %-12s  %s\n", i+1, it.Code, it.Name)
	}
	fmt.Fprintln(w)

	serverLabel := serverZoneLabel(rep.StartedAt)
		fmt.Fprintf(w, "扫描时间（北京时间）：%s\n", rep.StartedAt.In(beijingLocation).Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "扫描时间（服务器 · %s）：%s\n", serverLabel, rep.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "扫描标的数：%d\n", rep.Total)
	fmt.Fprintf(w, "命中数：%d\n", rep.Matched)
	fmt.Fprintf(w, "耗时：%s\n", rep.Elapsed)
	return nil
}

func serverZoneLabel(t time.Time) string {
	loc := t.Location()
	name := loc.String()
	if name != "" && name != "Local" {
		return name
	}
	if abbr, _ := t.Zone(); abbr != "" && abbr != "LMT" {
		return abbr
	}
	_, offset := t.Zone()
	hours := offset / 3600
	mins := (offset % 3600) / 60
	if mins == 0 {
		return fmt.Sprintf("UTC%+d", hours)
	}
	return fmt.Sprintf("UTC%+d:%02d", hours, mins)
}
