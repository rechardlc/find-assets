package crypto

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

const (
	defaultBinanceBaseURL = "https://fapi.binance.com"
	defaultOKXBaseURL     = "https://www.okx.com"
)

type BinanceSource struct {
	BaseURL string
	Client  *http.Client
	Top     int
}

func NewBinanceSource(top int) *BinanceSource {
	return &BinanceSource{BaseURL: defaultBinanceBaseURL, Client: http.DefaultClient, Top: top}
}

func (s *BinanceSource) Name() string { return "binance" }

func (s *BinanceSource) ListAssets(ctx context.Context) ([]Asset, error) {
	exchangeInfo, err := s.getBinanceExchangeInfo(ctx)
	if err != nil {
		return nil, err
	}
	tickers, err := s.getBinanceTickers(ctx)
	if err != nil {
		return nil, err
	}
	funding, _ := s.getBinanceFunding(ctx)

	metrics := make([]Metric, 0, len(tickers))
	for _, t := range tickers {
		info, ok := exchangeInfo[t.Symbol]
		if !ok {
			continue
		}
		metrics = append(metrics, Metric{
			Symbol:             t.Symbol,
			ExchangeSymbol:     t.Symbol,
			Base:               info.BaseAsset,
			Quote:              info.QuoteAsset,
			Exchange:           s.Name(),
			Status:             info.Status,
			PriceChangePercent: parseFloat(t.PriceChangePercent),
			High24h:            parseFloat(t.HighPrice),
			Low24h:             parseFloat(t.LowPrice),
			Open24h:            parseFloat(t.OpenPrice),
			QuoteVolume:        parseFloat(t.QuoteVolume),
			FundingRate:        funding[t.Symbol],
		})
	}
	return BuildHotAltPool(metrics, PoolOptions{Top: s.Top, ExcludeMajors: true}), nil
}

func (s *BinanceSource) Klines(ctx context.Context, asset Asset, interval string, limit int) ([]model.Kline, error) {
	if limit <= 0 {
		limit = 300
	}
	symbol := asset.ExchangeSymbol
	if symbol == "" {
		symbol = asset.Symbol
	}
	values := url.Values{}
	values.Set("symbol", symbol)
	values.Set("interval", interval)
	values.Set("limit", strconv.Itoa(limit))
	raw, err := s.get(ctx, "/fapi/v1/klines?"+values.Encode())
	if err != nil {
		return nil, err
	}
	return parseBinanceKlines(raw)
}

type binanceExchangeInfo struct {
	Symbols []binanceSymbol `json:"symbols"`
}

type binanceSymbol struct {
	Symbol       string `json:"symbol"`
	BaseAsset    string `json:"baseAsset"`
	QuoteAsset   string `json:"quoteAsset"`
	ContractType string `json:"contractType"`
	Status       string `json:"status"`
}

type binanceTicker struct {
	Symbol             string `json:"symbol"`
	PriceChangePercent string `json:"priceChangePercent"`
	OpenPrice          string `json:"openPrice"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	QuoteVolume        string `json:"quoteVolume"`
}

type binanceFunding struct {
	Symbol          string `json:"symbol"`
	LastFundingRate string `json:"lastFundingRate"`
}

func (s *BinanceSource) getBinanceExchangeInfo(ctx context.Context) (map[string]binanceSymbol, error) {
	raw, err := s.get(ctx, "/fapi/v1/exchangeInfo")
	if err != nil {
		return nil, err
	}
	var resp binanceExchangeInfo
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]binanceSymbol, len(resp.Symbols))
	for _, sym := range resp.Symbols {
		if sym.ContractType == "PERPETUAL" && sym.QuoteAsset == "USDT" {
			out[sym.Symbol] = sym
		}
	}
	return out, nil
}

func (s *BinanceSource) getBinanceTickers(ctx context.Context) ([]binanceTicker, error) {
	raw, err := s.get(ctx, "/fapi/v1/ticker/24hr")
	if err != nil {
		return nil, err
	}
	var out []binanceTicker
	return out, json.Unmarshal(raw, &out)
}

func (s *BinanceSource) getBinanceFunding(ctx context.Context) (map[string]float64, error) {
	raw, err := s.get(ctx, "/fapi/v1/premiumIndex")
	if err != nil {
		return nil, err
	}
	var rows []binanceFunding
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(rows))
	for _, row := range rows {
		out[row.Symbol] = parseFloat(row.LastFundingRate)
	}
	return out, nil
}

func (s *BinanceSource) get(ctx context.Context, path string) (json.RawMessage, error) {
	return httpGet(ctx, s.Client, strings.TrimRight(s.BaseURL, "/")+path)
}

type OKXSource struct {
	BaseURL string
	Client  *http.Client
	Top     int
}

func NewOKXSource(top int) *OKXSource {
	return &OKXSource{BaseURL: defaultOKXBaseURL, Client: http.DefaultClient, Top: top}
}

func (s *OKXSource) Name() string { return "okx" }

func (s *OKXSource) ListAssets(ctx context.Context) ([]Asset, error) {
	raw, err := s.get(ctx, "/api/v5/market/tickers?instType=SWAP")
	if err != nil {
		return nil, err
	}
	var resp okxTickerResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	metrics := make([]Metric, 0, len(resp.Data))
	for _, row := range resp.Data {
		if !strings.HasSuffix(row.InstID, "-USDT-SWAP") {
			continue
		}
		base := strings.TrimSuffix(row.InstID, "-USDT-SWAP")
		metrics = append(metrics, Metric{
			Symbol:             normalizeOKXSwapSymbol(row.InstID),
			ExchangeSymbol:     row.InstID,
			Base:               base,
			Quote:              "USDT",
			Exchange:           s.Name(),
			Status:             "TRADING",
			PriceChangePercent: okxChangePercent(row),
			High24h:            parseFloat(row.High24h),
			Low24h:             parseFloat(row.Low24h),
			Open24h:            parseFloat(row.Open24h),
			QuoteVolume:        parseFloat(row.VolCcy24h),
		})
	}
	return BuildHotAltPool(metrics, PoolOptions{Top: s.Top, ExcludeMajors: true}), nil
}

func (s *OKXSource) Klines(ctx context.Context, asset Asset, interval string, limit int) ([]model.Kline, error) {
	if limit <= 0 {
		limit = 300
	}
	instID := asset.ExchangeSymbol
	if instID == "" {
		instID = toOKXSwapSymbol(asset.Symbol)
	}
	values := url.Values{}
	values.Set("instId", instID)
	values.Set("bar", interval)
	values.Set("limit", strconv.Itoa(limit))
	raw, err := s.get(ctx, "/api/v5/market/candles?"+values.Encode())
	if err != nil {
		return nil, err
	}
	return parseOKXKlines(raw)
}

func (s *OKXSource) get(ctx context.Context, path string) (json.RawMessage, error) {
	return httpGet(ctx, s.Client, strings.TrimRight(s.BaseURL, "/")+path)
}

type okxTickerResponse struct {
	Code string      `json:"code"`
	Msg  string      `json:"msg"`
	Data []okxTicker `json:"data"`
}

type okxTicker struct {
	InstID    string `json:"instId"`
	Last      string `json:"last"`
	Open24h   string `json:"open24h"`
	High24h   string `json:"high24h"`
	Low24h    string `json:"low24h"`
	VolCcy24h string `json:"volCcy24h"`
}

func httpGet(ctx context.Context, client *http.Client, url string) (json.RawMessage, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "find-assets/crypto-scanner")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}

func parseBinanceKlines(raw json.RawMessage) ([]model.Kline, error) {
	var rows [][]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]model.Kline, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		out = append(out, model.Kline{
			Date:   time.UnixMilli(int64(row[0].(float64))),
			Open:   parseAnyFloat(row[1]),
			High:   parseAnyFloat(row[2]),
			Low:    parseAnyFloat(row[3]),
			Close:  parseAnyFloat(row[4]),
			Volume: int64(parseAnyFloat(row[5])),
		})
	}
	return out, nil
}

func parseOKXKlines(raw json.RawMessage) ([]model.Kline, error) {
	var resp struct {
		Code string  `json:"code"`
		Msg  string  `json:"msg"`
		Data [][]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if resp.Code != "" && resp.Code != "0" {
		return nil, fmt.Errorf("okx error %s: %s", resp.Code, resp.Msg)
	}
	out := make([]model.Kline, 0, len(resp.Data))
	for _, row := range resp.Data {
		if len(row) < 6 {
			continue
		}
		out = append(out, model.Kline{
			Date:   time.UnixMilli(int64(parseAnyFloat(row[0]))),
			Open:   parseAnyFloat(row[1]),
			High:   parseAnyFloat(row[2]),
			Low:    parseAnyFloat(row[3]),
			Close:  parseAnyFloat(row[4]),
			Volume: int64(parseAnyFloat(row[5])),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out, nil
}

func normalizeOKXSwapSymbol(instID string) string {
	return strings.ReplaceAll(strings.TrimSuffix(instID, "-SWAP"), "-", "")
}

func toOKXSwapSymbol(symbol string) string {
	symbol = strings.TrimSuffix(strings.ToUpper(symbol), "-SWAP")
	if strings.Contains(symbol, "-") {
		return symbol + "-SWAP"
	}
	return strings.TrimSuffix(symbol, "USDT") + "-USDT-SWAP"
}

func okxChangePercent(row okxTicker) float64 {
	open := parseFloat(row.Open24h)
	if open <= 0 {
		return 0
	}
	return (parseFloat(row.Last) - open) / open * 100
}

func parseAnyFloat(v any) float64 {
	switch x := v.(type) {
	case string:
		return parseFloat(x)
	case float64:
		return x
	default:
		return 0
	}
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
