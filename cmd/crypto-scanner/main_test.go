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
	if cfg.source != "binance,okx" {
		t.Fatalf("unexpected source: %q", cfg.source)
	}
	if cfg.pool != "hot_alt" || cfg.top != 20 {
		t.Fatalf("unexpected pool config: pool=%q top=%d", cfg.pool, cfg.top)
	}
	if cfg.interval != "15m" || cfg.pattern != "reversal" {
		t.Fatalf("unexpected strategy config: interval=%q pattern=%q", cfg.interval, cfg.pattern)
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
	if cfg.customFile != defaultCustomFile {
		t.Fatalf("unexpected custom file default: %q", cfg.customFile)
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
