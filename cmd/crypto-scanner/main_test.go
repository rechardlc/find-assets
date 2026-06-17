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
	if cfg.mailTo != "rechard.liu@qq.com" || cfg.mailFrom != "rechard.liu@qq.com" || cfg.smtpUser != "rechard.liu@qq.com" {
		t.Fatalf("unexpected mail defaults: to=%q from=%q user=%q", cfg.mailTo, cfg.mailFrom, cfg.smtpUser)
	}
	if cfg.smtpHost != "smtp.qq.com" || cfg.smtpPort != 465 {
		t.Fatalf("unexpected smtp defaults: host=%q port=%d", cfg.smtpHost, cfg.smtpPort)
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
