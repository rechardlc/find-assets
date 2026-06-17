package exporter

import (
	"bytes"
	"strings"
	"testing"
)

func TestConsoleWriteUsesMarketNeutralAssetLabel(t *testing.T) {
	var buf bytes.Buffer
	err := (Console{}).Write(&buf, &Report{Matched: 0, Elapsed: "1s"})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "符合条件的标的共计: 0 个") {
		t.Fatalf("expected market-neutral asset label, got:\n%s", out)
	}
	if strings.Contains(out, "股票") {
		t.Fatalf("output should not hard-code stock wording:\n%s", out)
	}
}
