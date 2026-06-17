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
		From: "rechard.liu@qq.com",
		To:   "rechard.liu@qq.com",
	}, &exporter.Report{
		Title:     "15分钟超跌拐点",
		Mode:      "15m:reversal",
		StartedAt: time.Date(2026, 6, 17, 10, 45, 0, 0, time.Local),
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
		"From: rechard.liu@qq.com",
		"To: rechard.liu@qq.com",
		"Subject: find-assets 命中提醒：15m:reversal 命中 1 个",
		"PEPEUSDT",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected email to contain %q, got:\n%s", want, text)
		}
	}
}
