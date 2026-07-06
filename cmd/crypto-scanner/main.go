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
	"github.com/find-assets/scanner/internal/crypto/reversal"
	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/notify"
	stocksource "github.com/find-assets/scanner/internal/source"
)

type config struct {
	source      string
	pool        string
	top         int
	intervals   []crypto.IntervalSpec
	bars        int
	workers     int
	schedule    bool
	delay       time.Duration
	exportArg   string
	outDir      string
	mail        bool
	mailTo      string
	mailFrom    string
	smtpHost    string
	smtpPort    int
	smtpUser    string
	smtpPass    string
	envFile     string
	custom      bool
	customFile  string
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	src, err := buildSource(cfg.source, cfg.top)
	if err != nil {
		log.Fatal(err)
	}
	svc := crypto.NewService(src)

	runInterval := func(interval string) {
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
		rep, err := svc.RunReversal(ctx, crypto.ScanJob{
			Interval:  interval,
			BarsLimit: cfg.bars,
			Workers:   cfg.workers,
			Assets:    assets,
		})
		if err != nil {
			log.Printf("[%s] 扫描失败: %v", interval, err)
			return
		}
		if rep == nil {
			opt := reversal.DefaultOptions(interval)
			log.Printf("[%s] K 线根数不足（需要至少 %d 根），跳过本周期", interval, reversal.MinRequiredBars(opt))
			return
		}
		label := "crypto_" + interval + "_reversal"
		if err := dispatchExports(rep, splitFormats(cfg.exportArg), cfg.outDir, label); err != nil {
			log.Printf("[%s] 导出失败: %v", interval, err)
		}
		if err := maybeSendMail(cfg, rep); err != nil {
			log.Printf("[%s] 邮件通知失败: %v", interval, err)
		}
	}

	if !cfg.schedule {
		for _, spec := range cfg.intervals {
			runInterval(spec.Name)
		}
		return
	}

	next := crypto.InitSchedule(time.Now(), cfg.intervals, cfg.delay)
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
			spec, ok := crypto.SpecByName(cfg.intervals, name)
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
	fs.StringVar(&cfg.source, "source", "binance,okx", "数据源回退顺序：binance,okx")
	fs.StringVar(&cfg.pool, "pool", "hot_alt", "合约池：hot_alt")
	fs.IntVar(&cfg.top, "top", 20, "每日缓存的候选合约数量")
	fs.StringVar(&intervalsArg, "intervals", "15m,1h,4h", "K 线周期列表，逗号分隔：15m,1h,4h")
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
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	intervals, err := crypto.ParseIntervalList(intervalsArg)
	if err != nil {
		return config{}, err
	}
	cfg.intervals = intervals
	return cfg, nil
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

func buildSource(spec string, top int) (crypto.Source, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(spec)), ",")
	sources := make([]crypto.Source, 0, len(parts))
	for _, part := range parts {
		switch strings.TrimSpace(part) {
		case "", "binance":
			sources = append(sources, crypto.NewBinanceSource(top))
		case "okx":
			sources = append(sources, crypto.NewOKXSource(top))
		default:
			return nil, fmt.Errorf("未知数字货币数据源: %s", part)
		}
	}
	return crypto.NewCompositeSource(sources...)
}

func loadOrBuildPool(ctx context.Context, src crypto.Source, pool string, top int) ([]crypto.Asset, error) {
	baseDir, err := stocksource.ExeDir()
	if err != nil {
		return nil, err
	}
	for _, exchange := range strings.Split(src.Name(), ",") {
		path, useCache, err := crypto.PreparePoolCacheAt(baseDir, pool, exchange)
		if err != nil {
			return nil, err
		}
		if !useCache {
			continue
		}
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
	exchange := src.Name()
	if strings.Contains(exchange, ",") {
		exchange = "unknown"
	}
	path, _, err := crypto.PreparePoolCacheAt(baseDir, pool, exchange)
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
