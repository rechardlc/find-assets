package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/find-assets/scanner/internal/crypto"
	"github.com/find-assets/scanner/internal/crypto/amplitude"
	"github.com/find-assets/scanner/internal/crypto/box"
	"github.com/find-assets/scanner/internal/crypto/reversal"
	"github.com/find-assets/scanner/internal/crypto/trend"
	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/notify"
	stocksource "github.com/find-assets/scanner/internal/source"
)

type config struct {
	source             string
	pool               string
	top                int
	intervals          []crypto.IntervalSpec
	pierceIntervals    []crypto.IntervalSpec
	amplitudeIntervals []crypto.IntervalSpec
	amplitudePct       float64
	boxIntervals       []crypto.IntervalSpec
	boxPct             float64
	boxLookback        int
	boxTouches         int
	boxMinGap          int
	boxAmplitudePct    float64
	boxSidewaysOnly    bool
	trend              bool
	trendGapPct        float64
	bars               int
	workers            int
	schedule           bool
	delay              time.Duration
	exportArg          string
	outDir             string
	mail               bool
	mailTo             string
	mailFrom           string
	smtpHost           string
	smtpPort           int
	smtpUser           string
	smtpPass           string
	envFile            string
	custom             bool
	customFile         string
	scanOnStart        bool
	rate               float64
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	src, err := buildSource(cfg.source, cfg.top, cfg.rate)
	if err != nil {
		log.Fatal(err)
	}
	if !cfg.custom {
		if err := clearPoolCacheOnStart(cfg.pool, src.Name()); err != nil {
			log.Fatal(err)
		}
	}
	svc := crypto.NewService(src)

	reversalSet := specNameSet(cfg.intervals)
	pierceSet := specNameSet(cfg.pierceIntervals)
	amplitudeSet := specNameSet(cfg.amplitudeIntervals)
	boxSet := specNameSet(cfg.boxIntervals)
	allSpecs := unionSpecs(cfg.intervals, cfg.pierceIntervals, cfg.amplitudeIntervals, cfg.boxIntervals)
	if cfg.trend {
		if _, ok := crypto.SpecByName(allSpecs, "1h"); !ok {
			h, err := crypto.ParseIntervalList("1h")
			if err != nil {
				log.Fatal(err)
			}
			allSpecs = unionSpecs(allSpecs, h)
		}
	}

	runInterval := func(interval string) {
		strategies := make([]string, 0, 4)
		if reversalSet[interval] {
			strategies = append(strategies, crypto.StrategyReversal)
		}
		if pierceSet[interval] {
			strategies = append(strategies, crypto.StrategyPierce)
		}
		if amplitudeSet[interval] {
			strategies = append(strategies, crypto.StrategyAmplitude)
		}
		if boxSet[interval] {
			strategies = append(strategies, crypto.StrategyBox)
		}
		if len(strategies) == 0 {
			return
		}

		var assets []crypto.Asset
		var err error
		if cfg.custom {
			assets, err = loadCustomAssets(cfg.customFile)
		} else {
			assets, err = loadOrBuildPool(ctx, src, cfg.pool, cfg.top)
		}
		if err != nil {
			log.Printf("[%s] 合约池准备失败: %v", interval, err)
			return
		}

		reps, err := svc.RunScan(ctx, crypto.ScanJob{
			Interval:        interval,
			BarsLimit:       cfg.bars,
			Workers:         cfg.workers,
			Assets:          assets,
			AmplitudePct:    cfg.amplitudePct,
			BoxPct:          cfg.boxPct,
			BoxLookback:     cfg.boxLookback,
			BoxTouches:      cfg.boxTouches,
			BoxMinGap:       cfg.boxMinGap,
			BoxAmplitudePct: cfg.boxAmplitudePct,
			BoxSidewaysOnly: cfg.boxSidewaysOnly,
		}, strategies)
		if err != nil {
			log.Printf("[%s] 扫描失败: %v", interval, err)
			return
		}

		if reversalSet[interval] && reps[crypto.StrategyReversal] == nil {
			opt := reversal.DefaultOptions(interval)
			log.Printf("[%s] K 线根数不足（需要至少 %d 根），跳过拐点", interval, reversal.MinRequiredBars(opt))
		}

		for _, stratName := range []string{crypto.StrategyReversal, crypto.StrategyPierce, crypto.StrategyAmplitude, crypto.StrategyBox} {
			rep := reps[stratName]
			if rep == nil {
				continue
			}
			label := "crypto_" + interval + "_" + stratName
			if err := dispatchExports(rep, splitFormats(cfg.exportArg), cfg.outDir, label); err != nil {
				log.Printf("[%s] 导出失败: %v", interval, err)
			}
			if err := maybeSendMail(cfg, rep); err != nil {
				log.Printf("[%s] 邮件通知失败: %v", interval, err)
			}
		}
	}

	runTrend := func() {
		if !cfg.trend {
			return
		}
		var assets []crypto.Asset
		var err error
		if cfg.custom {
			assets, err = loadCustomAssets(cfg.customFile)
		} else {
			assets, err = loadOrBuildPool(ctx, src, cfg.pool, cfg.top)
		}
		if err != nil {
			log.Printf("[trend] 合约池准备失败: %v", err)
			return
		}
		rep, err := svc.RunTrend(ctx, crypto.ScanJob{
			BarsLimit:      cfg.bars,
			Workers:        cfg.workers,
			Assets:         assets,
			TrendMinGapPct: cfg.trendGapPct,
		})
		if err != nil {
			log.Printf("[trend] 扫描失败: %v", err)
			return
		}
		if rep == nil {
			log.Printf("[trend] K 线根数不足，跳过")
			return
		}
		if err := dispatchExports(rep, splitFormats(cfg.exportArg), cfg.outDir, "crypto_1h_trend"); err != nil {
			log.Printf("[trend] 导出失败: %v", err)
		}
		if err := maybeSendMail(cfg, rep); err != nil {
			log.Printf("[trend] 邮件通知失败: %v", err)
		}
	}

	runAllOnce := func() {
		for _, spec := range allSpecs {
			runInterval(spec.Name)
		}
		runTrend()
	}

	if !cfg.schedule {
		runAllOnce()
		return
	}

	if cfg.scanOnStart {
		fmt.Println("启动即扫描一轮...")
		runAllOnce()
	}

	next := crypto.InitSchedule(time.Now(), allSpecs, cfg.delay)
	for {
		wakeAt := crypto.EarliestNext(next)
		fmt.Printf("下一次扫描时间: %s\n", wakeAt.Format("2006-01-02 15:04:05"))
		printUpcoming(next)

		timer := time.NewTimer(time.Until(wakeAt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		now := time.Now()
		due := crypto.DueIntervals(now, next)
		if len(due) == 0 {
			continue
		}
		fmt.Printf("触发周期: %s\n", strings.Join(due, ", "))

		for _, name := range due {
			runInterval(name)
			if name == "1h" {
				runTrend()
			}
			spec, ok := crypto.SpecByName(allSpecs, name)
			if !ok {
				continue
			}
			crypto.AdvanceInterval(now, spec, cfg.delay, next)
		}
	}
}

func printUpcoming(next map[string]time.Time) {
	names := make([]string, 0, len(next))
	for name := range next {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("  %s -> %s\n", name, next[name].Format("2006-01-02 15:04:05"))
	}
}

func parseConfig(args []string) (config, error) {
	envFile, explicitEnvFile := resolveEnvFileArg(args)
	if err := loadEnvFile(envFile, explicitEnvFile); err != nil {
		return config{}, err
	}

	fs := flag.NewFlagSet("crypto-scanner", flag.ContinueOnError)
	cfg := config{envFile: envFile}
	var intervalsArg string
	var pierceIntervalsArg string
	var amplitudeIntervalsArg string
	var boxIntervalsArg string
	// 数据源
	fs.StringVar(&cfg.source, "source", "okx", "数字货币数据源（当前仅支持 okx）")
	fs.StringVar(&cfg.pool, "pool", "hot_alt", "合约池：hot_alt")
	fs.IntVar(&cfg.top, "top", 300, "每日缓存的候选合约数量")
	// 拐点策略
	fs.StringVar(&intervalsArg, "intervals", "15m,1h,4h", "拐点策略 K 线周期列表，逗号分隔：15m,1h,4h")
	// 一箭穿心策略
	fs.StringVar(&pierceIntervalsArg, "pierce-intervals", "1h,4h", "一箭穿心策略 K 线周期列表，1h,4h逗号分隔；留空则关闭一箭穿心")
	// 情绪振幅异动策略
	fs.StringVar(&amplitudeIntervalsArg, "amplitude-intervals", "4h", "振幅异动策略 K 线周期列表，逗号分隔：15m,1h,4h；留空则关闭振幅异动")
	fs.Float64Var(&cfg.amplitudePct, "amplitude", amplitude.DefaultMinPct, "振幅异动阈值（百分比）：上一根 K 线 (最高-最低)/开盘 达到该值即命中")
	// 箱体震荡策略
	fs.StringVar(&boxIntervalsArg, "box-intervals", "4h", "箱体震荡策略 K 线周期列表，逗号分隔：15m,1h,4h；留空则关闭箱体震荡")
	fs.Float64Var(&cfg.boxPct, "box-pct", box.DefaultPct, "箱体带宽上限（百分比）：箱体内最高/最低价的最大相差幅度")
	fs.IntVar(&cfg.boxLookback, "box-lookback", box.DefaultLookback, "箱体震荡回看的已收盘 K 线根数")
	fs.IntVar(&cfg.boxTouches, "box-touches", box.DefaultTouches, "箱体震荡最少触及次数：几根 K 线踩在同一价位才算箱体")
	fs.IntVar(&cfg.boxMinGap, "box-min-gap", box.DefaultMinGap, "箱体首末两次触及之间的中间 K 线最少根数；1 表示仅要求首末触及不相邻")
	fs.Float64Var(&cfg.boxAmplitudePct, "box-amplitude", box.DefaultMinAmpPct, "箱体跨度内振幅下限（百分比）：跨度内 (最高-最低)/最低 需达到该值")
	fs.BoolVar(&cfg.boxSidewaysOnly, "box-sideways-only", false, "为 true 时仅输出顶底同时命中的窄幅横盘；默认 false 保留仅底/仅顶")
	// 趋势策略
	fs.BoolVar(&cfg.trend, "trend", true, "启用多周期趋势策略（15m+1h+4h 联合；每 1h 收盘扫描）")
	fs.Float64Var(&cfg.trendGapPct, "trend-gap", trend.DefaultMinGapPct, "多周期趋势 1h EMA 间距阈值（百分比），默认 1；影线须与 EMA30 交叉或相等；强势另要求间距 >8%")
	// 其他配置
	fs.IntVar(&cfg.bars, "bars", 300, "每个合约拉取的 K 线数量")
	fs.IntVar(&cfg.workers, "workers", 10, "最大并发数")
	fs.BoolVar(&cfg.schedule, "schedule", true, "按 K 线周期持续扫描；如需单次扫描可传 -schedule=false")
	fs.DurationVar(&cfg.delay, "delay", 20*time.Second, "K 线收盘后延迟执行")
	fs.StringVar(&cfg.exportArg, "export", "console", "导出格式列表，逗号分隔：console,json,md")
	fs.StringVar(&cfg.outDir, "out", "./output", "导出文件输出目录")
	fs.BoolVar(&cfg.mail, "mail", true, "命中时发送邮件通知")
	fs.StringVar(&cfg.mailTo, "mail-to", "richard_0525@foxmail.com", "邮件收件人")
	fs.StringVar(&cfg.mailFrom, "mail-from", "richard_0525@foxmail.com", "邮件发件人")
	fs.StringVar(&cfg.smtpHost, "smtp-host", "smtp.qq.com", "SMTP 服务器")
	fs.IntVar(&cfg.smtpPort, "smtp-port", 465, "SMTP 端口")
	fs.StringVar(&cfg.smtpUser, "smtp-user", "richard_0525@foxmail.com", "SMTP 用户名")
	fs.StringVar(&cfg.smtpPass, "smtp-pass", os.Getenv("FIND_ASSETS_SMTP_PASS"), "SMTP 授权码；建议使用 FIND_ASSETS_SMTP_PASS 环境变量")
	fs.StringVar(&cfg.envFile, "env", envFile, "环境变量文件路径；默认读取 .env")
	fs.BoolVar(&cfg.custom, "custom", false, "读取本地自定义数字货币列表；默认关闭")
	fs.StringVar(&cfg.customFile, "custom-file", defaultCustomFile, "本地自定义数字货币列表文件；一行一个交易对")
	fs.BoolVar(&cfg.scanOnStart, "scan-on-start", true, "定时模式下启动即先扫描一轮")
	fs.Float64Var(&cfg.rate, "rate", 15, "OKX 请求全局限速（次/秒）；<=0 表示不限速")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	intervals, err := crypto.ParseIntervalList(intervalsArg)
	if err != nil {
		return config{}, err
	}
	cfg.intervals = intervals

	if strings.TrimSpace(pierceIntervalsArg) != "" {
		pierceIntervals, err := crypto.ParseIntervalList(pierceIntervalsArg)
		if err != nil {
			return config{}, err
		}
		cfg.pierceIntervals = pierceIntervals
	}

	if strings.TrimSpace(amplitudeIntervalsArg) != "" {
		amplitudeIntervals, err := crypto.ParseIntervalList(amplitudeIntervalsArg)
		if err != nil {
			return config{}, err
		}
		cfg.amplitudeIntervals = amplitudeIntervals
	}
	if cfg.amplitudePct <= 0 {
		return config{}, fmt.Errorf("-amplitude 必须大于 0，当前为 %g", cfg.amplitudePct)
	}

	if strings.TrimSpace(boxIntervalsArg) != "" {
		boxIntervals, err := crypto.ParseIntervalList(boxIntervalsArg)
		if err != nil {
			return config{}, err
		}
		cfg.boxIntervals = boxIntervals
	}
	if cfg.boxPct <= 0 {
		return config{}, fmt.Errorf("-box-pct 必须大于 0，当前为 %g", cfg.boxPct)
	}
	if cfg.boxAmplitudePct <= 0 {
		return config{}, fmt.Errorf("-box-amplitude 必须大于 0，当前为 %g", cfg.boxAmplitudePct)
	}
	if cfg.trendGapPct <= 0 {
		return config{}, fmt.Errorf("-trend-gap 必须大于 0，当前为 %g", cfg.trendGapPct)
	}
	if cfg.boxTouches < box.DefaultTouches {
		return config{}, fmt.Errorf("-box-touches 至少为 %d，当前为 %d", box.DefaultTouches, cfg.boxTouches)
	}
	if cfg.boxMinGap < 1 {
		return config{}, fmt.Errorf("-box-min-gap 至少为 1，当前为 %d", cfg.boxMinGap)
	}
	if cfg.boxLookback < cfg.boxTouches {
		return config{}, fmt.Errorf("-box-lookback 不能小于 -box-touches（%d），当前为 %d", cfg.boxTouches, cfg.boxLookback)
	}
	if cfg.boxLookback < cfg.boxMinGap+2 {
		return config{}, fmt.Errorf("-box-lookback 需能容纳箱体跨度（-box-min-gap+2 = %d），当前为 %d", cfg.boxMinGap+2, cfg.boxLookback)
	}
	return cfg, nil
}

// specNameSet 返回周期名集合，便于按名判断某周期是否启用某策略。
func specNameSet(specs []crypto.IntervalSpec) map[string]bool {
	set := make(map[string]bool, len(specs))
	for _, s := range specs {
		set[s.Name] = true
	}
	return set
}

// unionSpecs 合并两组周期（按名去重），用于统一的调度时间表。
func unionSpecs(groups ...[]crypto.IntervalSpec) []crypto.IntervalSpec {
	seen := make(map[string]bool)
	out := make([]crypto.IntervalSpec, 0)
	for _, g := range groups {
		for _, s := range g {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			out = append(out, s)
		}
	}
	return out
}

func maybeSendMail(cfg config, rep *exporter.Report) error {
	if !cfg.mail || rep == nil || rep.Matched == 0 {
		return nil
	}
	err := notify.SendReport(notify.Config{
		Host:     cfg.smtpHost,
		Port:     cfg.smtpPort,
		User:     cfg.smtpUser,
		Password: cfg.smtpPass,
		From:     cfg.mailFrom,
		To:       cfg.mailTo,
	}, rep)
	if errors.Is(err, notify.ErrMissingPassword) {
		log.Println("邮件通知已跳过：未配置 SMTP 授权码，请设置 FIND_ASSETS_SMTP_PASS 或 -smtp-pass")
		return nil
	}
	return err
}

func buildSource(spec string, top int, ratePerSec float64) (crypto.Source, error) {
	for _, part := range strings.Split(strings.ToLower(strings.TrimSpace(spec)), ",") {
		switch strings.TrimSpace(part) {
		case "", "okx":
		default:
			return nil, fmt.Errorf("未知数字货币数据源: %s（当前仅支持 okx）", part)
		}
	}
	return crypto.NewOKXSourceWithRate(top, ratePerSec), nil
}

func clearPoolCacheOnStart(pool, exchange string) error {
	baseDir, err := stocksource.ExeDir()
	if err != nil {
		return err
	}
	if err := crypto.ClearTodayPoolCacheAt(baseDir, pool, exchange); err != nil {
		return err
	}
	fmt.Println("启动时已清除当日合约池缓存，将重新拉取")
	return nil
}

func loadOrBuildPool(ctx context.Context, src crypto.Source, pool string, top int) ([]crypto.Asset, error) {
	baseDir, err := stocksource.ExeDir()
	if err != nil {
		return nil, err
	}
	exchange := src.Name()
	path, useCache, err := crypto.PreparePoolCacheAt(baseDir, pool, exchange)
	if err != nil {
		return nil, err
	}
	if useCache {
		cache, err := crypto.LoadPoolCache(path)
		if err != nil {
			return nil, err
		}
		fmt.Printf("使用今日合约池缓存: %s\n", path)
		return cache.Assets, nil
	}

	assets, err := src.ListAssets(ctx)
	if err != nil {
		return nil, err
	}
	cache := crypto.PoolCache{
		Date:        time.Now().Format("2006-01-02"),
		Exchange:    exchange,
		Pool:        pool,
		Contract:    "usdt_perp",
		Top:         top,
		GeneratedAt: time.Now(),
		Assets:      assets,
	}
	if err := crypto.SavePoolCache(path, cache); err != nil {
		return nil, err
	}
	fmt.Printf("已保存今日合约池缓存: %s（%d 个）\n", path, len(assets))
	return assets, nil
}

func splitFormats(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"console"}
	}
	return out
}

func dispatchExports(rep anyReport, formats []string, outDir, label string) error {
	needFile := false
	for _, f := range formats {
		if f != "console" {
			needFile = true
			break
		}
	}
	if needFile {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
	}
	ts := repStart(rep).Format("20060102_150405")
	for _, f := range formats {
		exp := exporter.Get(f)
		if exp == nil {
			fmt.Fprintf(os.Stderr, "未知导出格式: %s\n", f)
			continue
		}
		if f == "console" {
			if err := exp.Write(os.Stdout, repReport(rep)); err != nil {
				return err
			}
			continue
		}
		path := filepath.Join(outDir, fmt.Sprintf("scan_%s_%s.%s", label, ts, f))
		fp, err := os.Create(path)
		if err != nil {
			return err
		}
		err = exp.Write(fp, repReport(rep))
		_ = fp.Close()
		if err != nil {
			return err
		}
		fmt.Printf("已导出 %s -> %s\n", f, path)
	}
	return nil
}

type anyReport = *exporter.Report

func repReport(rep anyReport) *exporter.Report { return rep }
func repStart(rep anyReport) time.Time         { return rep.StartedAt }
