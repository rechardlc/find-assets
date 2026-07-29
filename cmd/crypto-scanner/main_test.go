package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseConfigDefaultsToScheduledHotAltReversal(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.source != "okx" {
		t.Fatalf("unexpected source: %q", cfg.source)
	}
	if cfg.pool != "hot_alt" || cfg.top != 300 {
		t.Fatalf("unexpected pool config: pool=%q top=%d", cfg.pool, cfg.top)
	}
	if len(cfg.intervals) != 3 || cfg.intervals[0].Name != "15m" || cfg.intervals[1].Name != "1h" || cfg.intervals[2].Name != "4h" {
		t.Fatalf("unexpected intervals: %+v", cfg.intervals)
	}
	if !cfg.schedule {
		t.Fatal("expected schedule to be enabled by default")
	}
	if cfg.delay != 20*time.Second {
		t.Fatalf("unexpected delay: %s", cfg.delay)
	}
	if !cfg.mail {
		t.Fatal("expected mail notification enabled by default")
	}
	if cfg.mailTo != "richard_0525@foxmail.com" || cfg.mailFrom != "richard_0525@foxmail.com" || cfg.smtpUser != "richard_0525@foxmail.com" {
		t.Fatalf("unexpected mail defaults: to=%q from=%q user=%q", cfg.mailTo, cfg.mailFrom, cfg.smtpUser)
	}
	if cfg.smtpHost != "smtp.qq.com" || cfg.smtpPort != 465 {
		t.Fatalf("unexpected smtp defaults: host=%q port=%d", cfg.smtpHost, cfg.smtpPort)
	}
	if cfg.custom {
		t.Fatal("expected custom symbols to be disabled by default")
	}
	if !cfg.scanOnStart {
		t.Fatal("expected scan-on-start enabled by default")
	}
	if cfg.rate != 15 {
		t.Fatalf("unexpected rate default: %v", cfg.rate)
	}
	if cfg.customFile != defaultCustomFile {
		t.Fatalf("unexpected custom file default: %q", cfg.customFile)
	}
	if len(cfg.pierceIntervals) != 1 || cfg.pierceIntervals[0].Name != "4h" {
		t.Fatalf("unexpected pierce intervals default: %+v", cfg.pierceIntervals)
	}
}

func TestParseConfigDisablesPierceWithEmptyIntervals(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cfg, err := parseConfig([]string{"-pierce-intervals", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.pierceIntervals) != 0 {
		t.Fatalf("expected pierce disabled, got %+v", cfg.pierceIntervals)
	}
}

func TestParseConfigAmplitudeDefaults(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.amplitudeIntervals) != 1 || cfg.amplitudeIntervals[0].Name != "4h" {
		t.Fatalf("unexpected amplitude intervals default: %+v", cfg.amplitudeIntervals)
	}
	if cfg.amplitudePct != 9 {
		t.Fatalf("unexpected amplitude threshold default: %v", cfg.amplitudePct)
	}
}

func TestParseConfigAcceptsMultipleAmplitudeIntervals(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cfg, err := parseConfig([]string{"-amplitude-intervals", "1h,4h", "-amplitude", "12.5"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.amplitudeIntervals) != 2 || cfg.amplitudeIntervals[0].Name != "1h" || cfg.amplitudeIntervals[1].Name != "4h" {
		t.Fatalf("unexpected amplitude intervals: %+v", cfg.amplitudeIntervals)
	}
	if cfg.amplitudePct != 12.5 {
		t.Fatalf("unexpected amplitude threshold: %v", cfg.amplitudePct)
	}
}

func TestParseConfigDisablesAmplitudeWithEmptyIntervals(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cfg, err := parseConfig([]string{"-amplitude-intervals", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.amplitudeIntervals) != 0 {
		t.Fatalf("expected amplitude disabled, got %+v", cfg.amplitudeIntervals)
	}
}

func TestParseConfigRejectsNonPositiveAmplitude(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	if _, err := parseConfig([]string{"-amplitude", "0"}); err == nil {
		t.Fatal("expected error for non-positive amplitude threshold")
	}
}

func TestParseConfigBoxDefaults(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.boxIntervals) != 2 || cfg.boxIntervals[0].Name != "1h" || cfg.boxIntervals[1].Name != "4h" {
		t.Fatalf("unexpected box intervals default: %+v", cfg.boxIntervals)
	}
	if cfg.boxPct != 0.6 || cfg.boxLookback != 24 || cfg.boxTouches != 3 || cfg.boxMinGap != 6 {
		t.Fatalf("unexpected box defaults: pct=%v lookback=%d touches=%d min-gap=%d",
			cfg.boxPct, cfg.boxLookback, cfg.boxTouches, cfg.boxMinGap)
	}
}

func TestParseConfigDisablesBoxWithEmptyIntervals(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cfg, err := parseConfig([]string{"-box-intervals", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.boxIntervals) != 0 {
		t.Fatalf("expected box disabled, got %+v", cfg.boxIntervals)
	}
}

func TestParseConfigRejectsInvalidBoxOptions(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cases := map[string][]string{
		"non-positive width":     {"-box-pct", "0"},
		"touches below floor":    {"-box-touches", "2"},
		"lookback below touches": {"-box-lookback", "30", "-box-touches", "40"},
		"min gap below floor":    {"-box-min-gap", "0"},
		"lookback below span":    {"-box-lookback", "6", "-box-min-gap", "6"},
	}
	for name, args := range cases {
		if _, err := parseConfig(args); err == nil {
			t.Fatalf("%s: expected error for args %v", name, args)
		}
	}
}

func TestParseConfigLoadsSMTPPassFromEnvFile(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("FIND_ASSETS_SMTP_PASS=from-env-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := parseConfig([]string{"-env", envPath})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.envFile != envPath {
		t.Fatalf("expected env file %q, got %q", envPath, cfg.envFile)
	}
	if cfg.smtpPass != "from-env-file" {
		t.Fatalf("expected smtp pass from env file, got %q", cfg.smtpPass)
	}
}

func TestParseConfigEnablesCustomSymbolsFile(t *testing.T) {
	t.Setenv("FIND_ASSETS_SMTP_PASS", "")

	cfg, err := parseConfig([]string{"-custom=true", "-custom-file", "symbols.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.custom {
		t.Fatal("expected custom symbols to be enabled")
	}
	if cfg.customFile != "symbols.txt" {
		t.Fatalf("unexpected custom file: %q", cfg.customFile)
	}
}

func TestLoadCustomAssetsReadsOneSymbolPerLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "symbols.txt")
	content := "# custom crypto symbols\n\nbtcusdt\n ETHUSDT \nsolusdt\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	assets, err := loadCustomAssets(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 3 {
		t.Fatalf("expected 3 assets, got %d", len(assets))
	}
	want := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	for i, symbol := range want {
		if assets[i].Symbol != symbol {
			t.Fatalf("asset %d symbol: expected %q, got %q", i, symbol, assets[i].Symbol)
		}
		if assets[i].ExchangeSymbol != "" {
			t.Fatalf("asset %d exchange symbol should be empty for source-specific fallback, got %q", i, assets[i].ExchangeSymbol)
		}
		if assets[i].Quote != "USDT" {
			t.Fatalf("asset %d quote: expected USDT, got %q", i, assets[i].Quote)
		}
	}
}

func TestFormatAssetSymbolsUsesChineseDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "symbols.txt")
	if err := os.WriteFile(path, []byte("BTCUSDT\nETHUSDT\nSOLUSDT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assets, err := loadCustomAssets(path)
	if err != nil {
		t.Fatal(err)
	}

	got := formatAssetSymbols(assets)
	if got != "BTCUSDT、ETHUSDT、SOLUSDT" {
		t.Fatalf("unexpected symbols line: %q", got)
	}
}
