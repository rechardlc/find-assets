package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

const tencentKlineURL = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"

// Tencent 基于腾讯财经接口的 Source 实现，提供 K 线数据。
// 腾讯没有便捷的"全市场清单"接口，因此 ListAll 复用新浪源。
type Tencent struct {
	client *http.Client
	sina   *Sina
}

func NewTencent() *Tencent {
	return &Tencent{
		client: &http.Client{
			Timeout:   defaultTimeout,
			Transport: newHTTPTransport(),
		},
		sina: NewSina(),
	}
}

func (t *Tencent) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://gu.qq.com/")
}

func (t *Tencent) doGet(ctx context.Context, base string, q url.Values) ([]byte, error) {
	full := base
	if len(q) > 0 {
		full = base + "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	t.setHeaders(req)
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// ListAll 腾讯无公开全市场清单接口，回退到新浪。
func (t *Tencent) ListAll(ctx context.Context) ([]model.Stock, error) {
	return t.sina.ListAll(ctx)
}

// DailyKlines 通过腾讯接口拉取前复权日线。
// URL: https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=sh600000,day,,,300,qfq
// 响应：data.{symbol}.day = [[date, open, close, high, low, volume, ...], ...]
//
//	或 data.{symbol}.qfqday = [[...]] （带前复权时）
func (t *Tencent) DailyKlines(ctx context.Context, stock model.Stock, n int) ([]model.Kline, error) {
	symbol := sinaSymbol(stock)
	if symbol == "" {
		return nil, fmt.Errorf("无效的股票代码: %s", stock.Code)
	}
	q := url.Values{}
	q.Set("param", fmt.Sprintf("%s,day,,,%d,qfq", symbol, n))

	body, err := t.doGet(ctx, tencentKlineURL, q)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code int                                   `json:"code"`
		Data map[string]map[string][][]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析腾讯K线 %s: %w", stock.Code, err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("腾讯K线返回 code=%d", resp.Code)
	}
	dayData, ok := resp.Data[symbol]
	if !ok {
		return nil, fmt.Errorf("腾讯K线缺少 %s", symbol)
	}
	rows := dayData["qfqday"]
	if len(rows) == 0 {
		rows = dayData["day"]
	}

	out := make([]model.Kline, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		dateStr, _ := row[0].(string)
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		open := tencentFloat(row[1])
		clos := tencentFloat(row[2])
		high := tencentFloat(row[3])
		low := tencentFloat(row[4])
		vol := tencentInt(row[5])
		if open <= 0 || clos <= 0 || high <= 0 || low <= 0 {
			continue
		}
		out = append(out, model.Kline{
			Date: d, Open: open, Close: clos, High: high, Low: low, Volume: vol,
		})
	}
	return out, nil
}

func tencentFloat(v interface{}) float64 {
	switch x := v.(type) {
	case string:
		return parseFloat(x)
	case float64:
		return x
	}
	return 0
}

func tencentInt(v interface{}) int64 {
	switch x := v.(type) {
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return int64(f)
	case float64:
		return int64(x)
	}
	return 0
}
