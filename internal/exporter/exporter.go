package exporter

import (
	"io"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

// Report 用于序列化扫描结果，所有 Exporter 共用。
type Report struct {
	TaskID     string         `json:"task_id,omitempty"`
	Mode       string         `json:"mode"`
	Title      string         `json:"title"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	Elapsed    string         `json:"elapsed"`
	Total      int            `json:"total"`
	Matched    int            `json:"matched"`
	Results    []model.Result `json:"results"`
}

// Exporter 可将一次扫描结果写入任意 io.Writer。
type Exporter interface {
	Format() string
	ContentType() string
	Write(w io.Writer, rep *Report) error
}

// Registry 内置导出器注册表，按 format 名称索引。
func Registry() map[string]Exporter {
	return map[string]Exporter{
		"console": &Console{},
		"json":    &JSON{Pretty: true},
		"md":      &Markdown{},
	}
}

// Get 返回指定 format 的导出器，未知 format 返回 nil。
func Get(format string) Exporter {
	if e, ok := Registry()[format]; ok {
		return e
	}
	return nil
}
