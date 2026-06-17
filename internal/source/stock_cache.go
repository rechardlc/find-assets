package source

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const stockCacheDirName = "stocks"

var stockCacheFilePattern = regexp.MustCompile(`^stocks_(\d{8})\.json$`)

// ExeDir 返回当前可执行文件所在目录（解析符号链接）。
func ExeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return filepath.Dir(exe), nil
	}
	return filepath.Dir(exe), nil
}

// StockCacheDirAt 返回 baseDir 下的 stocks 缓存目录。
func StockCacheDirAt(baseDir string) string {
	return filepath.Join(baseDir, stockCacheDirName)
}

// TodayCacheFileName 返回今日缓存文件名（本地时区 YYYYMMDD）。
func TodayCacheFileName() string {
	return fmt.Sprintf("stocks_%s.json", time.Now().Format("20060102"))
}

// TodayCachePathAt 返回 baseDir/stocks/stocks_YYYYMMDD.json。
func TodayCachePathAt(baseDir string) string {
	return filepath.Join(StockCacheDirAt(baseDir), TodayCacheFileName())
}

// PrepareStockCache 初始化 exe 旁 stocks 目录：删除非今日缓存，返回今日路径及是否可直接读缓存。
func PrepareStockCache() (todayPath string, useCache bool, err error) {
	baseDir, err := ExeDir()
	if err != nil {
		return "", false, err
	}
	return PrepareStockCacheAt(baseDir)
}

// PrepareStockCacheAt 在 baseDir/stocks 下维护日缓存，并清理过期文件。
func PrepareStockCacheAt(baseDir string) (todayPath string, useCache bool, err error) {
	cacheDir := StockCacheDirAt(baseDir)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", false, err
	}

	today := time.Now().Format("20060102")
	todayPath = TodayCachePathAt(baseDir)

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return "", false, err
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		m := stockCacheFilePattern.FindStringSubmatch(ent.Name())
		if m == nil {
			continue
		}
		if m[1] != today {
			_ = os.Remove(filepath.Join(cacheDir, ent.Name()))
		}
	}

	if _, err := os.Stat(todayPath); err == nil {
		return todayPath, true, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	return todayPath, false, nil
}

// CacheFileDate 从缓存文件名解析日期，格式不符时返回空字符串。
func CacheFileDate(name string) string {
	m := stockCacheFilePattern.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return ""
	}
	return m[1]
}
