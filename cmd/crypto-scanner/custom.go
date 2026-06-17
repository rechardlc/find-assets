package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/find-assets/scanner/internal/crypto"
)

const defaultCustomFile = "./crypto_symbols.txt"

func loadCustomAssets(path string) ([]crypto.Asset, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("自定义数字货币文件路径不能为空")
	}

	fp, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	var assets []crypto.Asset
	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		symbol := strings.ToUpper(strings.TrimSpace(scanner.Text()))
		if symbol == "" || strings.HasPrefix(symbol, "#") {
			continue
		}
		assets = append(assets, crypto.Asset{
			Symbol: symbol,
			Name:   symbol,
			Base:   strings.TrimSuffix(symbol, "USDT"),
			Quote:  "USDT",
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("自定义数字货币文件 %s 未包含有效交易对", path)
	}
	return assets, nil
}

func formatAssetSymbols(assets []crypto.Asset) string {
	symbols := make([]string, 0, len(assets))
	for _, asset := range assets {
		symbol := strings.TrimSpace(asset.Symbol)
		if symbol != "" {
			symbols = append(symbols, symbol)
		}
	}
	return strings.Join(symbols, "、")
}
