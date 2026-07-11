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

	"golang.org/x/time/rate"

	"github.com/find-assets/scanner/internal/model"
)

const (
	defaultOKXBaseURL   = "https://www.okx.com"
	defaultHTTPTimeout  = 10 * time.Second
	defaultHTTPRetries  = 2
	defaultRetryBackoff = 500 * time.Millisecond
	// defaultRatePerSec 是 OKX 公共接口的默认全局限速（次/秒）。OKX 蜡烛图接口
	// 大约为每 IP 20~40 次 / 2 秒，取偏保守的中间值并留退避余量。
	defaultRatePerSec = 15
)

// newHTTPClient 返回带超时的 HTTP client，避免交易所卡住导致 goroutine 长期挂起。
func newHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultHTTPTimeout}
}

// newRateLimiter 创建每秒请求数的全局限速器；ratePerSec <= 0 表示不限速（返回 nil）。
func newRateLimiter(ratePerSec float64) *rate.Limiter {
	if ratePerSec <= 0 {
		return nil
	}
	burst := int(ratePerSec)
	if burst < 1 {
		burst = 1
	}
	return rate.NewLimiter(rate.Limit(ratePerSec), burst)
}

type OKXSource struct {
	BaseURL string
	Client  *http.Client
	Top     int

	limiter *rate.Limiter
}

func NewOKXSource(top int) *OKXSource {
	return NewOKXSourceWithRate(top, defaultRatePerSec)
}

// NewOKXSourceWithRate 创建带自定义全局限速（次/秒）的源；ratePerSec <= 0 表示不限速。
func NewOKXSourceWithRate(top int, ratePerSec float64) *OKXSource {
	return &OKXSource{
		BaseURL: defaultOKXBaseURL,
		Client:  newHTTPClient(),
		Top:     top,
		limiter: newRateLimiter(ratePerSec),
	}
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
	if resp.Code != "" && resp.Code != "0" {
		return nil, fmt.Errorf("okx error %s: %s", resp.Code, resp.Msg)
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
	values.Set("bar", MapIntervalForExchange(s.Name(), interval))
	values.Set("limit", strconv.Itoa(limit))
	raw, err := s.get(ctx, "/api/v5/market/candles?"+values.Encode())
	if err != nil {
		return nil, err
	}
	return parseOKXKlines(raw)
}

func (s *OKXSource) get(ctx context.Context, path string) (json.RawMessage, error) {
	return httpGet(ctx, s.Client, s.limiter, strings.TrimRight(s.BaseURL, "/")+path)
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

// httpGet 发送 GET 请求，先经全局限速器，再对网络错误、429 与 5xx 做有限次指数退避重试。
func httpGet(ctx context.Context, client *http.Client, limiter *rate.Limiter, url string) (json.RawMessage, error) {
	if client == nil {
		client = newHTTPClient()
	}
	var lastErr error
	for attempt := 0; attempt <= defaultHTTPRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * defaultRetryBackoff):
			}
		}
		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				return nil, err
			}
		}
		b, status, err := doGet(ctx, client, url)
		if err != nil {
			lastErr = err
			continue
		}
		if status == http.StatusTooManyRequests || status >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", status, string(b))
			continue
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("HTTP %d: %s", status, string(b))
		}
		return b, nil
	}
	return nil, lastErr
}

func doGet(ctx context.Context, client *http.Client, url string) (json.RawMessage, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "find-assets/crypto-scanner")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
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
