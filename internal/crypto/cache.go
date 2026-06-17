package crypto

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const poolCacheDirName = "crypto/pools"

var poolCacheFilePattern = regexp.MustCompile(`^(.+)_(\d{8})_(.+)\.json$`)

func PoolCacheDirAt(baseDir string) string {
	return filepath.Join(baseDir, filepath.FromSlash(poolCacheDirName))
}

func TodayPoolCachePathAt(baseDir, pool, exchange string) string {
	name := fmt.Sprintf("%s_%s_%s.json", pool, time.Now().Format("20060102"), exchange)
	return filepath.Join(PoolCacheDirAt(baseDir), name)
}

func PreparePoolCacheAt(baseDir, pool, exchange string) (todayPath string, useCache bool, err error) {
	cacheDir := PoolCacheDirAt(baseDir)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", false, err
	}

	today := time.Now().Format("20060102")
	todayPath = TodayPoolCachePathAt(baseDir, pool, exchange)

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return "", false, err
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		m := poolCacheFilePattern.FindStringSubmatch(ent.Name())
		if m == nil {
			continue
		}
		if m[1] == pool && m[3] == exchange && m[2] != today {
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

func SavePoolCache(path string, cache PoolCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func LoadPoolCache(path string) (PoolCache, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return PoolCache{}, err
	}
	var cache PoolCache
	if err := json.Unmarshal(b, &cache); err != nil {
		return PoolCache{}, err
	}
	return cache, nil
}
