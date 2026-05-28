package source

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/find-assets/scanner/internal/model"
)

const (
	defaultTimeout = 6 * time.Second
	maxRetries     = 1
	// listPageSize 单页拉取条数。
	// 三层防御 L1（化整为零）：放弃 pz=6000 大包，固定为 200，
	// 让单包流量贴近常规网页翻页规模，避开 WAF 异常流量审计。
	listPageSize = 200
	// pageJitterMinMS / pageJitterMaxMS 翻页间随机休眠区间（毫秒）。
	// 三层防御 L3（动态扰动）：每页落地后必须随机停 200~500ms，
	// 打散固定频率特征，避免被判定为机器人。
	pageJitterMinMS = 200
	pageJitterMaxMS = 500
)

var (
	// 东方财富 push2 多节点；网络不稳定时会随机分配到不同主机。
	listHosts = []string{
		"https://82.push2.eastmoney.com/api/qt/clist/get",
		"https://80.push2.eastmoney.com/api/qt/clist/get",
		"https://48.push2.eastmoney.com/api/qt/clist/get",
		"https://push2.eastmoney.com/api/qt/clist/get",
	}
	klineURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
)

// EastMoney 是基于东方财富 push2 接口的 Source 实现。
// 内置三层防限流策略：
//  1. 分页化整为零（listPageSize=200）；
//  2. 高仿真浏览器请求头注入（setRequestHeaders）；
//  3. 翻页动态扰动延迟（PageJitter / defaultPageJitter）。
type EastMoney struct {
	client     *http.Client
	userAgents []string
	listBase   string // 首次成功的清单接口节点，后续分页复用
	// pageJitterFn 翻页间的随机延迟生成器。
	// 生产环境默认 [200ms, 500ms]；单测场景可在同包内设置返回 0 以提速。
	pageJitterFn func() time.Duration
}

// pageJitter 取一次翻页随机间隔。若未注入则使用默认 [200ms,500ms]。
func (e *EastMoney) pageJitter() time.Duration {
	if e.pageJitterFn == nil {
		return defaultPageJitter()
	}
	return e.pageJitterFn()
}

func newHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		ForceAttemptHTTP2:   false,
		TLSNextProto:        make(map[string]func(string, *tls.Conn) http.RoundTripper),
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		MaxIdleConns:        300,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}
}

// defaultPageJitter 翻页之间的默认动态扰动延迟：[200ms, 500ms]。
// 三层防御 L3：随机化间隔以打散固定频率特征。
func defaultPageJitter() time.Duration {
	span := pageJitterMaxMS - pageJitterMinMS + 1
	return time.Duration(pageJitterMinMS+rand.Intn(span)) * time.Millisecond
}

// NewEastMoney 构造一个针对全市场优化过的 HTTP 客户端。
func NewEastMoney() *EastMoney {
	return &EastMoney{
		client: &http.Client{
			Timeout:   defaultTimeout,
			Transport: newHTTPTransport(),
		},
		// 三层防御 L2：UA 池覆盖 Win/Mac Chrome 与 Edge，
		// 彻底剔除 Go-http-client/1.1 这一高危特征。
		userAgents: []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
		},
		pageJitterFn: defaultPageJitter,
	}
}

func (e *EastMoney) ua() string {
	if len(e.userAgents) == 0 {
		return "Mozilla/5.0"
	}
	return e.userAgents[rand.Intn(len(e.userAgents))]
}

func (e *EastMoney) listBases() []string {
	if e.listBase == "" {
		return listHosts
	}
	out := make([]string, 0, len(listHosts))
	out = append(out, e.listBase)
	for _, base := range listHosts {
		if base != e.listBase {
			out = append(out, base)
		}
	}
	return out
}

func (e *EastMoney) setRequestHeaders(req *http.Request) {
	req.Header.Set("User-Agent", e.ua())
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", "https://quote.eastmoney.com/center/gridlist.html")
	req.Header.Set("Origin", "https://quote.eastmoney.com")
}

type listItem struct {
	F12 string `json:"f12"` // 代码
	F14 string `json:"f14"` // 名称
}

// parseListDiff 解析 clist 接口的 diff 字段。
// 东方财富在不同 np / 版本下可能返回 JSON 数组或 {"0":{...},"1":{...}} 对象。
func parseListDiff(raw json.RawMessage) ([]listItem, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []listItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}
	var keyed map[string]listItem
	if err := json.Unmarshal(raw, &keyed); err != nil {
		return nil, err
	}
	keys := make([]int, 0, len(keyed))
	for k := range keyed {
		i, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		keys = append(keys, i)
	}
	sort.Ints(keys)
	items = make([]listItem, 0, len(keys))
	for _, i := range keys {
		items = append(items, keyed[strconv.Itoa(i)])
	}
	return items, nil
}

func (e *EastMoney) listQuery(page, pageSize int) url.Values {
	q := url.Values{}
	q.Set("pn", strconv.Itoa(page))
	q.Set("pz", strconv.Itoa(pageSize))
	q.Set("po", "1")
	q.Set("np", "1")
	q.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	q.Set("wbp2u", "|0|0|0|web")
	q.Set("fltt", "2")
	q.Set("invt", "2")
	q.Set("fid", "f3")
	q.Set("fs", "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048")
	q.Set("fields", "f12,f14")
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	return q
}

const listPageRetries = 1

func (e *EastMoney) fetchListPage(ctx context.Context, page, pageSize int) ([]byte, error) {
	q := e.listQuery(page, pageSize)
	var lastErr error
	for attempt := 0; attempt < listPageRetries; attempt++ {
		if attempt > 0 {
			e.listBase = ""
			backoff := time.Duration(800*(1<<attempt))*time.Millisecond +
				time.Duration(rand.Intn(400))*time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		body, err := e.doGetAny(ctx, e.listBases(), q, &e.listBase)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown http error")
	}
	return nil, lastErr
}

// ListAll 拉取全市场清单（沪深 A 股）。
func (e *EastMoney) ListAll(ctx context.Context) ([]model.Stock, error) {
	return e.listAllEastMoney(ctx)
}

func (e *EastMoney) listAllEastMoney(ctx context.Context) ([]model.Stock, error) {
	all := make([]model.Stock, 0, 6000)
	var expectedTotal int
	rawFetched := 0

	for page := 1; ; page++ {
		// 三层防御 L3：除首页外，每页之间执行 [200ms,500ms] 随机休眠，
		// 把高频批量行为伪装成普通用户的高仿真翻页。
		if page > 1 {
			pause := e.pageJitter()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(pause):
			}
		}

		body, err := e.fetchListPage(ctx, page, listPageSize)
		if err != nil {
			return nil, fmt.Errorf("拉取股票清单失败 page=%d: %w（请检查网络或设置 HTTPS_PROXY）", page, err)
		}

		var resp struct {
			Data *struct {
				Total int             `json:"total"`
				Diff  json.RawMessage `json:"diff"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("解析股票清单失败 page=%d: %w", page, err)
		}
		if resp.Data == nil {
			break
		}
		if resp.Data.Total > expectedTotal {
			expectedTotal = resp.Data.Total
		}

		items, err := parseListDiff(resp.Data.Diff)
		if err != nil {
			return nil, fmt.Errorf("解析股票清单 diff page=%d: %w", page, err)
		}
		if len(items) == 0 {
			break
		}
		rawFetched += len(items)
		for _, it := range items {
			st := model.Stock{Code: it.F12, Name: it.F14}
			if IsTradable(st) {
				all = append(all, st)
			}
		}
		if expectedTotal > 0 && rawFetched >= expectedTotal {
			break
		}
		// 安全兜底，避免极端情况下无限翻页
		if page > 60 {
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

// doGetAny 依次尝试多个 base URL，成功后可选记录 sticky base。
func (e *EastMoney) doGetAny(ctx context.Context, bases []string, q url.Values, sticky *string) ([]byte, error) {
	var lastErr error
	for _, base := range bases {
		body, err := e.doGet(ctx, base, q)
		if err == nil {
			if sticky != nil {
				*sticky = base
			}
			return body, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown http error")
	}
	return nil, lastErr
}

// doGet 带超时 / UA / 退避重试的 GET。
func (e *EastMoney) doGet(ctx context.Context, base string, q url.Values) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(300*(1<<attempt))*time.Millisecond +
				time.Duration(rand.Intn(200))*time.Millisecond
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
		e.setRequestHeaders(req)

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
