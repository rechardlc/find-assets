package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

var (
	sinaListURL          = "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/Market_Center.getHQNodeData"
	sinaKlineURL         = "https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData"
	sinaKlineURLOverride string // 测试覆盖；非空时优先使用
)

func currentSinaKlineURL() string {
	if sinaKlineURLOverride != "" {
		return sinaKlineURLOverride
	}
	return sinaKlineURL
}

// Sina 基于新浪财经接口的 Source 实现，作为东方财富不可用时的备用源。
type Sina struct {
	client *http.Client
}

func NewSina() *Sina {
	return &Sina{
		client: &http.Client{
			Timeout:   defaultTimeout,
			Transport: newHTTPTransport(),
		},
	}
}

func (s *Sina) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://finance.sina.com.cn/")
}

func (s *Sina) doGet(ctx context.Context, base string, q url.Values) ([]byte, error) {
	full := base
	if len(q) > 0 {
		full = base + "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// ListAll 通过新浪行情接口拉取沪深 A 股清单。
func (s *Sina) ListAll(ctx context.Context) ([]model.Stock, error) {
	const pageSize = 100
	all := make([]model.Stock, 0, 6000)

	for page := 1; page <= 80; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("num", strconv.Itoa(pageSize))
		q.Set("sort", "symbol")
		q.Set("asc", "1")
		q.Set("node", "hs_a")
		q.Set("symbol", "")
		q.Set("_s_r_a", "page")

		body, err := s.doGet(ctx, sinaListURL, q)
		if err != nil {
			return nil, fmt.Errorf("新浪清单 page=%d: %w", page, err)
		}

		var items []struct {
			Code string `json:"code"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("解析新浪清单 page=%d: %w", page, err)
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			st := model.Stock{Code: it.Code, Name: it.Name}
			if IsTradable(st) {
				all = append(all, st)
			}
		}
		if len(items) < pageSize {
			break
		}
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("新浪清单为空")
	}
	return all, nil
}

// DailyKlines 通过新浪 K 线接口拉取前复权日线。
// 返回示例：[{"day":"2025-11-20","open":"8.10","high":"8.15","low":"8.02","close":"8.10","volume":"1234567"}, ...]
func (s *Sina) DailyKlines(ctx context.Context, stock model.Stock, n int) ([]model.Kline, error) {
	symbol := sinaSymbol(stock)
	if symbol == "" {
		return nil, fmt.Errorf("无效的股票代码: %s", stock.Code)
	}
	q := url.Values{}
	q.Set("symbol", symbol)
	q.Set("scale", "240")
	q.Set("ma", "no")
	q.Set("datalen", strconv.Itoa(n))

	body, err := s.doGet(ctx, currentSinaKlineURL(), q)
	if err != nil {
		return nil, err
	}
	body = sinaTrimCallback(body)

	var rows []struct {
		Day    string `json:"day"`
		Open   string `json:"open"`
		High   string `json:"high"`
		Low    string `json:"low"`
		Close  string `json:"close"`
		Volume string `json:"volume"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("解析新浪K线失败 %s: %w", stock.Code, err)
	}

	out := make([]model.Kline, 0, len(rows))
	for _, r := range rows {
		t, err := time.Parse("2006-01-02", r.Day)
		if err != nil {
			continue
		}
		o := parseFloat(r.Open)
		c := parseFloat(r.Close)
		h := parseFloat(r.High)
		l := parseFloat(r.Low)
		if o <= 0 || c <= 0 || h <= 0 || l <= 0 {
			continue
		}
		out = append(out, model.Kline{
			Date: t, Open: o, Close: c, High: h, Low: l, Volume: parseInt(r.Volume),
		})
	}
	return out, nil
}

// sinaSymbol 将 6 位代码转换为新浪格式：sh600000 / sz000001。
func sinaSymbol(s model.Stock) string {
	if len(s.Code) != 6 {
		return ""
	}
	switch s.Code[0] {
	case '6':
		return "sh" + s.Code
	case '0', '3':
		return "sz" + s.Code
	case '8', '4':
		return "bj" + s.Code
	}
	return ""
}

func sinaTrimCallback(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	if i := strings.Index(s, "["); i > 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			return []byte(s[i : j+1])
		}
	}
	return b
}
