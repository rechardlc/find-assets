package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

const (
	listURL  = "https://80.push2.eastmoney.com/api/qt/clist/get"
	klineURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"

	defaultTimeout = 8 * time.Second
	maxRetries     = 2
)

// EastMoney 是基于东方财富 push2 接口的 Source 实现。
type EastMoney struct {
	client     *http.Client
	userAgents []string
}

// NewEastMoney 构造一个针对全市场优化过的 HTTP 客户端。
func NewEastMoney() *EastMoney {
	return &EastMoney{
		client: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        300,
				MaxIdleConnsPerHost: 300,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
		userAgents: []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		},
	}
}

func (e *EastMoney) ua() string {
	if len(e.userAgents) == 0 {
		return "Mozilla/5.0"
	}
	return e.userAgents[rand.Intn(len(e.userAgents))]
}

// ListAll 拉取全市场清单（沪深 A 股）。
func (e *EastMoney) ListAll(ctx context.Context) ([]model.Stock, error) {
	const pageSize = 500
	all := make([]model.Stock, 0, 6000)
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("pn", strconv.Itoa(page))
		q.Set("pz", strconv.Itoa(pageSize))
		q.Set("po", "1")
		q.Set("np", "1")
		q.Set("fltt", "2")
		q.Set("invt", "2")
		q.Set("fid", "f3")
		// 沪深 A 股全集：m:0+t:6 / m:0+t:80 / m:1+t:2 / m:1+t:23 / m:0+t:81+s:2048
		q.Set("fs", "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048")
		q.Set("fields", "f12,f14")
		q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))

		body, err := e.doGet(ctx, listURL, q)
		if err != nil {
			return nil, fmt.Errorf("拉取股票清单失败 page=%d: %w", page, err)
		}

		var resp struct {
			Data *struct {
				Total int `json:"total"`
				Diff  []struct {
					F12 string `json:"f12"` // 代码
					F14 string `json:"f14"` // 名称
				} `json:"diff"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("解析股票清单失败 page=%d: %w", page, err)
		}
		if resp.Data == nil || len(resp.Data.Diff) == 0 {
			break
		}
		for _, it := range resp.Data.Diff {
			st := model.Stock{Code: it.F12, Name: it.F14}
			if IsTradable(st) {
				all = append(all, st)
			}
		}
		if len(resp.Data.Diff) < pageSize {
			break
		}
		// 安全兜底，避免极端情况下无限翻页
		if page > 40 {
			break
		}
	}
	return all, nil
}

// DailyKlines 取一只股票最近 n 根前复权日线。
func (e *EastMoney) DailyKlines(ctx context.Context, stock model.Stock, n int) ([]model.Kline, error) {
	q := url.Values{}
	q.Set("secid", stock.SecID())
	q.Set("fields1", "f1,f2,f3,f4,f5,f6")
	q.Set("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61")
	q.Set("klt", "101") // 日线
	q.Set("fqt", "1")   // 前复权
	q.Set("end", "20500101")
	q.Set("lmt", strconv.Itoa(n))
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))

	body, err := e.doGet(ctx, klineURL, q)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data *struct {
			Klines []string `json:"klines"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析K线失败 %s: %w", stock.Code, err)
	}
	if resp.Data == nil {
		return nil, nil
	}
	out := make([]model.Kline, 0, len(resp.Data.Klines))
	for _, line := range resp.Data.Klines {
		k, ok := parseKline(line)
		if !ok {
			continue
		}
		out = append(out, k)
	}
	return out, nil
}

// parseKline 解析单根 K 线字符串。
// 格式（fields2 对应）：date,open,close,high,low,volume,amount,amplitude,pct,chg,turnover
func parseKline(line string) (model.Kline, bool) {
	parts := strings.Split(line, ",")
	if len(parts) < 6 {
		return model.Kline{}, false
	}
	date, err := time.Parse("2006-01-02", parts[0])
	if err != nil {
		return model.Kline{}, false
	}
	open := parseFloat(parts[1])
	clos := parseFloat(parts[2])
	high := parseFloat(parts[3])
	low := parseFloat(parts[4])
	vol := parseInt(parts[5])

	if open <= 0 || clos <= 0 || high <= 0 || low <= 0 {
		return model.Kline{}, false
	}
	return model.Kline{
		Date: date, Open: open, Close: clos, High: high, Low: low, Volume: vol,
	}, true
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// doGet 带超时 / UA / 退避重试的 GET。
func (e *EastMoney) doGet(ctx context.Context, base string, q url.Values) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(200*(1<<attempt))*time.Millisecond +
				time.Duration(rand.Intn(120))*time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", e.ua())
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("Referer", "https://quote.eastmoney.com/")

		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			continue
		}
		return body, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown http error")
	}
	return nil, lastErr
}
