package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/find-assets/scanner/internal/model"
)

func TestFileList_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stocks.json")
	if err := os.WriteFile(path, []byte(`[
		{"code":"600000","name":"浦发银行"},
		{"code":"000001","name":"平安银行"},
		{"code":"123456","name":"未知"},
		{"code":"600001","name":"*ST 测试"}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	fl := NewFileList(path)
	stocks, err := fl.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stocks) != 2 {
		t.Fatalf("want 2 tradable stocks (ST 与无效代码被过滤), got %d", len(stocks))
	}
}

func TestFileList_CSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stocks.csv")
	if err := os.WriteFile(path, []byte("code,name\n600000,浦发\n# 注释\n000001,平安\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stocks, err := NewFileList(path).ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stocks) != 2 {
		t.Fatalf("want 2 stocks, got %d", len(stocks))
	}
}

func TestSaveStocks_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	in := []model.Stock{{Code: "600000", Name: "浦发"}, {Code: "000001", Name: "平安"}}
	if err := SaveStocks(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := NewFileList(path).ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Code != "600000" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}
