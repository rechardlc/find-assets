package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/model"
	"github.com/find-assets/scanner/internal/server"
	"github.com/find-assets/scanner/internal/service"
	"github.com/find-assets/scanner/internal/source"
	"github.com/find-assets/scanner/internal/strategy"
)

func main() {
	var (
		help       = flag.Bool("help", false, "显示帮助信息并退出")
		period     = flag.String("period", "day", "K 线周期：day | week （CLI 模式必填）")
		pattern    = flag.String("pattern", "pierce", "选股形态：pierce 一箭穿心 | reversal 超跌拐点")
		workers    = flag.Int("workers", 100, "最大并发数")
		barsLimit  = flag.Int("bars", 600, "拉取日线根数")
		rangeArg   = flag.Float64("range", 2, "[pierce 形态] 均线粘合度阈值，百分比单位，默认 2 (=2%)")
		volumeArg  = flag.Float64("volume", 5, "[pierce 形态] 放量阈值，百分比单位，默认 5 (=较前一根成交量增加 20%)")
		exportArg  = flag.String("export", "console", "导出格式列表，逗号分隔：console,json,md")
		outDir     = flag.String("out", "./output", "导出文件输出目录")
		serve      = flag.Bool("serve", false, "以 HTTP 服务模式运行")
		addr       = flag.String("addr", ":8080", "HTTP 监听地址")
		srcSpec = flag.String("source", "auto", "数据源：auto | em | sina | tencent | file:./path.json，可逗号串联做回退（如 file:./stocks.json,em）")
	)
	// 自定义用法：-h / -help 以及参数错误时统一打印结构化帮助。
	flag.Usage = func() { printHelp(os.Stderr) }
	os.Args = expandShortFlags(os.Args)
	flag.Parse()

	if *help {
		printHelp(os.Stdout)
		return
	}

	if *serve {
		composite, err := source.NewComposite(*srcSpec)
		if err != nil {
			fmt.Fprintln(os.Stderr, "数据源配置错误:", err)
			os.Exit(2)
		}
		runServer(service.New(composite), *addr)
		return
	}

	if *period == "" || *pattern == "" {
		fmt.Fprintln(os.Stderr, "请使用 -p=day|week 与 -pt=pierce|reversal 指定策略；或 -s 启动 HTTP 服务")
		flag.Usage()
		os.Exit(2)
	}

	todayPath, useCache, err := source.PrepareStockCache()
	if err != nil {
		fmt.Fprintln(os.Stderr, "清单缓存初始化失败:", err)
		os.Exit(2)
	}

	spec := *srcSpec
	var saveCachePath string
	if useCache {
		spec = "file:" + todayPath + "," + spec
		fmt.Printf("使用今日缓存清单: %s\n", todayPath)
	} else {
		saveCachePath = todayPath
		fmt.Println("今日无缓存，将从远程拉取清单...")
	}

	composite, err := source.NewComposite(spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "数据源配置错误:", err)
		os.Exit(2)
	}

	runCLI(service.New(composite), *period, *pattern, *workers, *barsLimit, *rangeArg, *volumeArg, *exportArg, *outDir, saveCachePath)
}

// ---------------- CLI 模式 ----------------

func runCLI(svc *service.ScanService, period, pattern string, workers, bars int, rangePct, volumePct float64, exportArg, outDir, saveCachePath string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("★ find-assets 启动，周期: %s，形态: %s ★\n", period, pattern)
	if pattern == "pierce" {
		fmt.Printf("   均线粘合度阈值: %.2f%% (range)\n", rangePct)
		fmt.Printf("   放量阈值: %.2f%% (volume)\n", volumePct)
	}
	fmt.Println("1. 正在拉取全市场股票清单...")

	var totalStocks int
	var fetchedStocks []model.Stock
	var nextProgress int64 = 1000

	rep, err := svc.Run(ctx, service.Params{
		Period: period, Pattern: pattern, Workers: workers, BarsLimit: bars,
		Range: rangePct, Volume: volumePct, TaskID: uuid.NewString(),
		OnStocks: func(total int, stocks []model.Stock) {
			totalStocks = total
			fetchedStocks = stocks
			active := ""
			if c, ok := svc.Source().(*source.Composite); ok {
				if n := c.ActiveName(); n != "" {
					active = "（数据源: " + n + "）"
				}
			}
			fmt.Printf("成功获取到 %d 只股票%s，开始并发扫描...\n", total, active)
		},
		Progress: func(done, total int64) {
			threshold := atomic.LoadInt64(&nextProgress)
			if done >= threshold {
				if atomic.CompareAndSwapInt64(&nextProgress, threshold, threshold+1000) {
					fmt.Printf("已扫描 %d/%d 只股票...\n", done, total)
				}
			}
		},
	})
	if err != nil {
		log.Fatalf("扫描失败: %v\n提示：请检查网络连接；今日首次运行需成功拉取远程清单。", err)
	}
	_ = totalStocks

	if saveCachePath != "" && len(fetchedStocks) > 0 {
		if err := source.SaveStocks(saveCachePath, fetchedStocks); err != nil {
			log.Fatalf("保存清单缓存失败: %v", err)
		}
		fmt.Printf("已保存今日清单缓存: %s（%d 只）\n", saveCachePath, len(fetchedStocks))
	}

	formats := splitFormats(exportArg)
	if err := dispatchExports(rep, formats, outDir, period+"_"+pattern); err != nil {
		log.Fatalf("导出失败: %v", err)
	}
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
		out = append(out, "console")
	}
	return out
}

func dispatchExports(rep *exporter.Report, formats []string, outDir, label string) error {
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
	ts := rep.StartedAt.Format("20060102_150405")
	for _, f := range formats {
		exp := exporter.Get(f)
		if exp == nil {
			fmt.Fprintf(os.Stderr, "未知导出格式: %s\n", f)
			continue
		}
		if f == "console" {
			if err := exp.Write(os.Stdout, rep); err != nil {
				return err
			}
			continue
		}
		path := filepath.Join(outDir, fmt.Sprintf("scan_%s_%s.%s", label, ts, f))
		fp, err := os.Create(path)
		if err != nil {
			return err
		}
		err = exp.Write(fp, rep)
		_ = fp.Close()
		if err != nil {
			return err
		}
		fmt.Printf("已导出 %s -> %s\n", f, path)
	}
	return nil
}

// ---------------- 帮助信息 ----------------

// printHelp 输出结构化的帮助：用法、周期、形态、组合、参数与示例。
// 周期/形态列表由 strategy 包动态生成，新增维度会自动出现。
func printHelp(w io.Writer) {
	exe := filepath.Base(os.Args[0])

	fmt.Fprintf(w, "find-assets —— 全市场多维度量化选股器\n\n")
	fmt.Fprintf(w, "策略 = 周期(period) × 形态(pattern) 自由组合。\n\n")

	fmt.Fprintf(w, "用法:\n")
	fmt.Fprintf(w, "  %s -p <周期> -pt <形态> [选项]   # CLI 扫描（可用完整名 -period/-pattern）\n", exe)
	fmt.Fprintf(w, "  %s -s [-a=:8080]                 # 启动 HTTP 服务\n", exe)
	fmt.Fprintf(w, "  %s -h                            # 显示本帮助\n\n", exe)

	fmt.Fprintf(w, "周期 (-p / -period):\n")
	for _, p := range strategy.Periods() {
		fmt.Fprintf(w, "  %-10s %s\n", p.Name, p.Label)
	}
	fmt.Fprintf(w, "\n形态 (-pt / -pattern):\n")
	for _, p := range strategy.Patterns() {
		fmt.Fprintf(w, "  %-10s %s\n", p.Name, p.Label)
	}

	fmt.Fprintf(w, "\n可用组合:\n")
	for _, pd := range strategy.Periods() {
		for _, pt := range strategy.Patterns() {
			fmt.Fprintf(w, "  -p=%-5s -pt=%-9s %s\n", pd.Name, pt.Name, pd.Label+pt.Label)
		}
	}

	fmt.Fprintf(w, "\n选项 (简写 / 完整名):\n")
	fmt.Fprintf(w, "  -p,  -period string      K 线周期 (默认 day)\n")
	fmt.Fprintf(w, "  -pt, -pattern string     选股形态 (默认 pierce)\n")
	fmt.Fprintf(w, "  -w,  -workers int        最大并发数 (默认 100)\n")
	fmt.Fprintf(w, "  -b,  -bars int           拉取日线根数 (默认 600)\n")
	fmt.Fprintf(w, "  -r,  -range float        [pierce] 粘合度阈值%% (默认 2)\n")
	fmt.Fprintf(w, "  -v,  -volume float       [pierce] 放量阈值%% (默认 20)\n")
	fmt.Fprintf(w, "  -e,  -export string      导出格式 console,json,md (默认 console)\n")
	fmt.Fprintf(w, "  -o,  -out string         文件导出目录 (默认 ./output)\n")
	fmt.Fprintf(w, "  -s,  -serve              以 HTTP 服务模式运行\n")
	fmt.Fprintf(w, "  -a,  -addr string        HTTP 监听地址 (默认 :8080)\n")
	fmt.Fprintf(w, "  -so, -source string      数据源 (默认 auto)\n")
	fmt.Fprintf(w, "  -h,  -help               显示帮助并退出\n")
	fmt.Fprintf(w, "\n清单缓存 (CLI 自动):\n")
	fmt.Fprintf(w, "  可执行文件旁 stocks/ 目录按日落盘；同日复用缓存，跨日拉远程并覆盖。\n")

	fmt.Fprintf(w, "\n示例:\n")
	fmt.Fprintf(w, "  %s -p=day -pt=pierce\n", exe)
	fmt.Fprintf(w, "  %s -p day -pt pierce -r 1.2 -v 25\n", exe)
	fmt.Fprintf(w, "  %s -p week -pt reversal -e json,md -o ./output\n", exe)
	fmt.Fprintf(w, "  %s -p week -pt pierce              # 周线一箭穿心\n", exe)
	fmt.Fprintf(w, "  %s -s -a :8080\n", exe)
}

// ---------------- HTTP 服务模式 ----------------

func runServer(svc *service.ScanService, addr string) {
	router, handler := server.NewRouter(svc)
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		fmt.Printf("★ HTTP 服务已启动: http://%s/api/v1 ★\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Println("\n正在关闭服务...")
	handler.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
