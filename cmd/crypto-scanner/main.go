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
	"strings"
	"syscall"
	"time"

	"github.com/find-assets/scanner/internal/crypto"
	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/notify"
	stocksource "github.com/find-assets/scanner/internal/source"
)

type config struct {
	source     string
	pool       string
	top        int
	interval   string
	pattern    string
	bars       int
	workers    int
	schedule   bool
	delay      time.Duration
	exportArg  string
	outDir     string
	mail       bool
	mailTo     string
	mailFrom   string
	smtpHost   string
	smtpPort   int
	smtpUser   string
	smtpPass   string
	envFile    string
	custom     bool
	customFile string
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

	run := func() {
		var assets []crypto.Asset
		var err error
		if cfg.custom {
			assets, err = loadCustomAssets(cfg.customFile)
		} else {
			assets, err = loadOrBuildPool(ctx, src, cfg.pool, cfg.top)
		}
		if err != nil {
			log.Printf("合约池准备失败: %v", err)
			return
		}
		rep, err := svc.Run(ctx, crypto.Params{
			Interval:  cfg.interval,
			Pattern:   cfg.pattern,
			BarsLimit: cfg.bars,
			Workers:   cfg.workers,
			Assets:    assets,
		})
		if err != nil {
			log.Printf("扫描失败: %v", err)
			return
		}
		if err := dispatchExports(rep, splitFormats(cfg.exportArg), cfg.outDir, "crypto_"+cfg.interval+"_"+cfg.pattern); err != nil {
			log.Printf("导出失败: %v", err)
		}
		if err := maybeSendMail(cfg, rep); err != nil {
			log.Printf("邮件通知失败: %v", err)
		}
	}

	if !cfg.schedule {
		run()
		return
	}

	intervalDuration, err := time.ParseDuration(cfg.interval)
	if err != nil {
		log.Fatalf("无法解析 interval %q: %v", cfg.interval, err)
	}
	for {
		next := crypto.NextDelayedRun(time.Now(), intervalDuration, cfg.delay)
		fmt.Printf("下一次扫描时间: %s\n", next.Format("2006-01-02 15:04:05"))
		if cfg.custom {
			assets, err := loadCustomAssets(cfg.customFile)
			if err != nil {
				log.Printf("自定义币种读取失败: %v", err)
			} else {
				fmt.Printf("扫描币种: %s\n", formatAssetSymbols(assets))
			}
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			run()
		}
	}
}

func parseConfig(args []string) (config, error) {
	envFile, explicitEnvFile := resolveEnvFileArg(args)
	if err := loadEnvFile(envFile, explicitEnvFile); err != nil {
		return config{}, err
	}

	fs := flag.NewFlagSet("crypto-scanner", flag.ContinueOnError)
	cfg := config{envFile: envFile}
	fs.StringVar(&cfg.source, "source", "binance,okx", "数据源回退顺序：binance,okx")
	fs.StringVar(&cfg.pool, "pool", "hot_alt", "合约池：hot_alt")
	fs.IntVar(&cfg.top, "top", 20, "每日缓存的候选合约数量")
	fs.StringVar(&cfg.interval, "interval", "15m", "K 线周期")
	fs.StringVar(&cfg.pattern, "pattern", "reversal", "策略形态")
	fs.IntVar(&cfg.bars, "bars", 300, "每个合约拉取的 K 线数量")
	fs.IntVar(&cfg.workers, "workers", 10, "最大并发数")
	fs.BoolVar(&cfg.schedule, "schedule", true, "按 K 线周期持续扫描；如需单次扫描可传 -schedule=false")
	fs.DurationVar(&cfg.delay, "delay", 20*time.Second, "K 线收盘后延迟执行")
	fs.StringVar(&cfg.exportArg, "export", "console", "导出格式列表，逗号分隔：console,json,md")
	fs.StringVar(&cfg.outDir, "out", "./output", "导出文件输出目录")
	fs.BoolVar(&cfg.mail, "mail", true, "命中时发送邮件通知")
	fs.StringVar(&cfg.mailTo, "mail-to", "rechard.liu@qq.com", "邮件收件人")
	fs.StringVar(&cfg.mailFrom, "mail-from", "rechard.liu@qq.com", "邮件发件人")
	fs.StringVar(&cfg.smtpHost, "smtp-host", "smtp.qq.com", "SMTP 服务器")
	fs.IntVar(&cfg.smtpPort, "smtp-port", 465, "SMTP 端口")
	fs.StringVar(&cfg.smtpUser, "smtp-user", "rechard.liu@qq.com", "SMTP 用户名")
	fs.StringVar(&cfg.smtpPass, "smtp-pass", os.Getenv("FIND_ASSETS_SMTP_PASS"), "SMTP 授权码；建议使用 FIND_ASSETS_SMTP_PASS 环境变量")
	fs.StringVar(&cfg.envFile, "env", envFile, "环境变量文件路径；默认读取 .env")
	fs.BoolVar(&cfg.custom, "custom", false, "读取本地自定义数字货币列表；默认关闭")
	fs.StringVar(&cfg.customFile, "custom-file", defaultCustomFile, "本地自定义数字货币列表文件；一行一个交易对")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
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
