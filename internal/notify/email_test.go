package notify

import (
	"strings"
	"testing"
	"time"

	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/model"
)

func TestBuildReportEmailIncludesQQRecipientAndMatches(t *testing.T) {
	msg, err := BuildReportEmail(Config{
		From: "richard_0525@foxmail.com",
		To:   "richard_0525@foxmail.com",
	}, &exporter.Report{
		AssetClass: exporter.AssetCrypto,
		Title:      "15分钟超跌拐点",
		Mode:       "15m:reversal",
		StartedAt:  time.Date(2026, 6, 17, 10, 45, 0, 0, time.Local),
		Total:     20,
		Matched:   1,
		Results: []model.Result{
			{Code: "PEPEUSDT", Name: "PEPE USDT Perpetual"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(msg)
	for _, want := range []string{
		"From: richard_0525@foxmail.com",
		"To: richard_0525@foxmail.com",
		"Subject: 命中提醒：15分钟超跌拐点 命中 1 个",
		"数字货币 · 15分钟超跌拐点",
		"命中清单",
		"PEPE USDT Perpetual",
		"扫描时间（服务器 ·",
		"扫描时间（北京时间）：2026-06-17 10:45:00",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected email to contain %q, got:\n%s", want, text)
		}
	}

	if strings.Contains(text, "PEPEUSDT") {
		t.Fatalf("expected name to replace code in hit list, got:\n%s", text)
	}

	hitIdx := strings.Index(text, "命中清单")
	metaIdx := strings.Index(text, "扫描时间（服务器 ·")
	if hitIdx < 0 || metaIdx < 0 || hitIdx > metaIdx {
		t.Fatalf("expected 命中清单 before scan metadata, got:\n%s", text)
	}
}

func TestBuildReportEmailFallsBackToCodeWhenNameEmpty(t *testing.T) {
	msg, err := BuildReportEmail(Config{
		From: "a@example.com",
		To:   "b@example.com",
	}, &exporter.Report{
		Title:     "日线一箭穿心",
		Mode:      "day:pierce",
		StartedAt: time.Date(2026, 6, 17, 10, 45, 0, 0, time.UTC),
		Matched:   1,
		Results: []model.Result{
			{Code: "600519", Name: "  "},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(msg), "600519") {
		t.Fatalf("expected code fallback when name empty, got:\n%s", msg)
	}
}
