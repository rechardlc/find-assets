package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareStockCacheAt_RemovesStaleAndDetectsToday(t *testing.T) {
	base := t.TempDir()
	cacheDir := StockCacheDirAt(base)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	todayPath := TodayCachePathAt(base)
	stalePath := filepath.Join(cacheDir, "stocks_20200101.json")
	if err := os.WriteFile(stalePath, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(todayPath, []byte(`[{"code":"600000","name":"浦发"}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	gotPath, useCache, err := PrepareStockCacheAt(base)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != todayPath {
		t.Fatalf("todayPath = %q, want %q", gotPath, todayPath)
	}
	if !useCache {
		t.Fatal("expected useCache true when today's file exists")
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale cache should be removed, stat err=%v", err)
	}
}

func TestPrepareStockCacheAt_NoTodayNeedsRemote(t *testing.T) {
	base := t.TempDir()
	gotPath, useCache, err := PrepareStockCacheAt(base)
	if err != nil {
		t.Fatal(err)
	}
	if useCache {
		t.Fatal("expected useCache false when no cache")
	}
	wantPath := TodayCachePathAt(base)
	if gotPath != wantPath {
		t.Fatalf("todayPath = %q, want %q", gotPath, wantPath)
	}
	if _, err := os.Stat(StockCacheDirAt(base)); err != nil {
		t.Fatalf("stocks dir should be created: %v", err)
	}
}

func TestCacheFileDate(t *testing.T) {
	if got := CacheFileDate("stocks_20260606.json"); got != "20260606" {
		t.Fatalf("got %q", got)
	}
	if got := CacheFileDate("other.json"); got != "" {
		t.Fatalf("got %q", got)
	}
}
