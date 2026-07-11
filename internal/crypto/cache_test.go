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

	oldPath := filepath.Join(oldDir, "hot_alt_20000101_okx.json")
	if err := os.WriteFile(oldPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	todayPath := TodayPoolCachePathAt(dir, "hot_alt", "okx")
	if err := os.WriteFile(todayPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, use, err := PreparePoolCacheAt(dir, "hot_alt", "okx")
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

func TestClearTodayPoolCacheAtRemovesTodayFile(t *testing.T) {
	dir := t.TempDir()
	cacheDir := PoolCacheDirAt(dir)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	todayPath := TodayPoolCachePathAt(dir, "hot_alt", "okx")
	if err := os.WriteFile(todayPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(cacheDir, "hot_alt_20000101_okx.json")
	if err := os.WriteFile(otherPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ClearTodayPoolCacheAt(dir, "hot_alt", "okx"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(todayPath); !os.IsNotExist(err) {
		t.Fatalf("expected today cache removed, stat err=%v", err)
	}
	if _, err := os.Stat(otherPath); err != nil {
		t.Fatalf("expected other-day cache kept, stat err=%v", err)
	}

	_, use, err := PreparePoolCacheAt(dir, "hot_alt", "okx")
	if err != nil {
		t.Fatal(err)
	}
	if use {
		t.Fatal("expected useCache false after clearing today cache")
	}
}
