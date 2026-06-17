package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparePoolCacheAtReusesTodayAndRemovesOldPoolCaches(t *testing.T) {
	dir := t.TempDir()
	oldDir := PoolCacheDirAt(dir)
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldPath := filepath.Join(oldDir, "hot_alt_20000101_binance.json")
	if err := os.WriteFile(oldPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	todayPath := TodayPoolCachePathAt(dir, "hot_alt", "binance")
	if err := os.WriteFile(todayPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, use, err := PreparePoolCacheAt(dir, "hot_alt", "binance")
	if err != nil {
		t.Fatal(err)
	}
	if !use || got != todayPath {
		t.Fatalf("expected reuse today cache %q, got %q use=%v", todayPath, got, use)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old cache removed, stat err=%v", err)
	}
}
