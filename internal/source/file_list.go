package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/find-assets/scanner/internal/model"
)

// FileList 从本地 JSON / CSV 文件加载股票清单。
// 适用于：
//   - 网络受限时离线扫描
//   - 通过 `-cache-list` 把上次成功的拉取保存为本地缓存
type FileList struct {
	path string
}

// NewFileList 创建一个文件清单源。文件不需要在构造时存在，调用 ListAll 时再校验。
func NewFileList(path string) *FileList {
	return &FileList{path: path}
}

// ListAll 从文件读取清单。
// 支持两种格式：
//   - JSON：[{"code":"600000","name":"浦发银行"}, ...]
//   - CSV ：每行 `code,name`，列顺序固定（无表头或有表头均可）
func (f *FileList) ListAll(_ context.Context) ([]model.Stock, error) {
	if f.path == "" {
		return nil, fmt.Errorf("未指定清单文件路径")
	}
	b, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("读取清单文件 %s: %w", f.path, err)
	}
	ext := strings.ToLower(filepath.Ext(f.path))
	switch ext {
	case ".json":
		return parseStockJSON(b)
	case ".csv", ".txt", "":
		return parseStockCSV(b)
	default:
		return parseStockJSON(b)
	}
}

// DailyKlines 文件源不提供 K 线，应由 Composite 自动回退到在线源。
func (f *FileList) DailyKlines(context.Context, model.Stock, int) ([]model.Kline, error) {
	return nil, fmt.Errorf("file 源不支持 K 线，请配合在线源使用")
}

func parseStockJSON(b []byte) ([]model.Stock, error) {
	var items []model.Stock
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("解析 JSON 清单: %w", err)
	}
	out := make([]model.Stock, 0, len(items))
	for _, st := range items {
		if IsTradable(st) {
			out = append(out, st)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("清单为空或全部被过滤")
	}
	return out, nil
}

func parseStockCSV(b []byte) ([]model.Stock, error) {
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	out := make([]model.Stock, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		code := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if code == "code" {
			continue
		}
		st := model.Stock{Code: code, Name: name}
		if IsTradable(st) {
			out = append(out, st)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("清单为空或全部被过滤")
	}
	return out, nil
}

// SaveStocks 把股票清单保存为 JSON 文件，便于下次离线加载。
func SaveStocks(path string, stocks []model.Stock) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stocks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
